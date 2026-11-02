package gitsource

import (
	"encoding/base64"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/Khaymah/dhow/internal/gitref"
)

const (
	NoSSHCommand      = "/dev/null"
	FixedPath         = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
	CheckoutUser      = "x-access-token"
	MaxKnownHostBytes = 16 << 10
	sendEnvOption     = "SendEnv=GIT_PROTOCOL"
	uploadPack        = "git-upload-pack"
)

var (
	ErrSSHCommand = errors.New("the ssh command must be an absolute path of plain characters")
	ErrWant       = errors.New("a fetch wants a full ref or a commit id")
	ErrKnownHost  = errors.New("the known host line must be the repository host pattern, a key type and a base64 key")
	ErrSSHUse     = errors.New("git asked ssh for something other than fetching this repository")
	ErrNotSSH     = errors.New("the repository is not fetched over ssh")
)

func knownHostTypes() []string {
	return []string{"ssh-ed25519", "ecdsa-sha2-nistp256", "ecdsa-sha2-nistp384", "ecdsa-sha2-nistp521", "rsa-sha2-256", "rsa-sha2-512", "ssh-rsa"}
}

func safePathPattern(p string) bool {
	if !strings.HasPrefix(p, "/") || len(p) > MaxPathBytes || strings.Contains(p, "..") {
		return false
	}
	for i := 0; i < len(p); i++ {
		c := p[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '/' && c != '-' && c != '_' && c != '.' {
			return false
		}
	}
	return true
}

func SafeConfig() []string {
	return safeConfig(NoSSHCommand)
}

func CheckoutConfig(sshCommand string) ([]string, error) {
	if !safePathPattern(sshCommand) {
		return nil, ErrSSHCommand
	}
	return safeConfig(sshCommand), nil
}

func safeConfig(sshCommand string) []string {
	return []string{
		"--literal-pathspecs",
		"--no-replace-objects",
		"-c", "protocol.allow=never",
		"-c", "protocol.https.allow=always",
		"-c", "protocol.ssh.allow=always",
		"-c", "core.hooksPath=/dev/null",
		"-c", "core.sshCommand=" + sshCommand,
		"-c", "core.fsmonitor=false",
		"-c", "credential.helper=",
		"-c", "init.templateDir=",
		"-c", "submodule.recurse=false",
		"-c", "fetch.recurseSubmodules=false",
		"-c", "http.followRedirects=false",
		"-c", "transfer.fsckObjects=true",
		"-c", "gc.auto=0",
		"-c", "maintenance.auto=false",
	}
}

type EnvOptions struct {
	Home       string
	Askpass    string
	HTTPSProxy string
}

func Env(o EnvOptions) []string {
	env := []string{
		"PATH=" + FixedPath,
		"HOME=" + o.Home,
		"LC_ALL=C",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_LFS_SKIP_SMUDGE=1",
		"GIT_SSH_VARIANT=ssh",
	}
	if o.Askpass != "" {
		env = append(env, "GIT_ASKPASS="+o.Askpass)
	}
	if o.HTTPSProxy != "" {
		env = append(env, "HTTPS_PROXY="+o.HTTPSProxy, "https_proxy="+o.HTTPSProxy)
	}
	return env
}

func CheckWant(want string) error {
	if gitref.ValidSha(want) {
		return nil
	}
	if _, err := gitref.Parse(want); err != nil {
		return errors.Join(ErrWant, err)
	}
	return nil
}

func FetchArgsWith(config []string, u URL, want string, depth int) ([]string, error) {
	if u.Transport != TransportHTTPS && u.Transport != TransportSSH {
		return nil, ErrNotFetched
	}
	if _, err := Parse(u.String()); err != nil {
		return nil, err
	}
	if err := CheckWant(want); err != nil {
		return nil, err
	}
	if depth < 1 || depth > MaxDepth {
		return nil, ErrDepth
	}
	return append(slices.Clone(config), "fetch", "--quiet", "--depth="+strconv.Itoa(depth), "--no-tags", "--", u.String(), want), nil
}

func DeepenArgs(config []string, u URL, want string, depth int) ([]string, error) {
	args, err := FetchArgsWith(config, u, want, depth)
	if err != nil {
		return nil, err
	}
	i := slices.Index(args, "--no-tags")
	return slices.Insert(args, i, "--filter=tree:0"), nil
}

func DeepenSteps() []int { return []int{64, 1024, MaxDepth} }

func FetchArgs(u URL, ref string, depth int) ([]string, error) {
	if _, err := gitref.Parse(ref); err != nil {
		return nil, err
	}
	return FetchArgsWith(SafeConfig(), u, ref, depth)
}

func InitArgs(config []string) []string {
	return append(slices.Clone(config), "init", "--quiet")
}

func FetchHeadArgs(config []string) []string {
	return append(slices.Clone(config), "rev-parse", "--verify", "--end-of-options", "FETCH_HEAD^{commit}")
}

func IsAncestorArgs(config []string, sha string) ([]string, error) {
	if err := gitref.CheckSha(sha); err != nil {
		return nil, err
	}
	return append(slices.Clone(config), "merge-base", "--is-ancestor", "--end-of-options", sha, "FETCH_HEAD"), nil
}

func CheckoutArgs(config []string, sha string) ([]string, error) {
	if err := gitref.CheckSha(sha); err != nil {
		return nil, err
	}
	return append(slices.Clone(config), "checkout", "--quiet", "--force", "--detach", sha, "--"), nil
}

func CheckKnownHost(u URL, line string) error {
	if u.Transport != TransportSSH {
		return ErrNotSSH
	}
	if line == "" || len(line) > MaxKnownHostBytes || strings.ContainsAny(line, "\r\n\x00") {
		return ErrKnownHost
	}
	fields := strings.Split(line, " ")
	if len(fields) < 3 || fields[0] != u.KnownHostsPattern() || !slices.Contains(knownHostTypes(), fields[1]) {
		return ErrKnownHost
	}
	blob, err := base64.StdEncoding.DecodeString(fields[2])
	if err != nil || len(blob) < 4+len(fields[1]) || string(blob[4:4+len(fields[1])]) != fields[1] {
		return ErrKnownHost
	}
	return nil
}

type SSHOptions struct {
	Binary         string
	KeyFile        string
	KnownHostsFile string
	ProxyCommand   string
}

func SSHArgs(u URL, o SSHOptions, command string) ([]string, error) {
	if u.Transport != TransportSSH {
		return nil, ErrNotSSH
	}
	for _, p := range []string{o.Binary, o.KeyFile, o.KnownHostsFile} {
		if !safePathPattern(p) {
			return nil, ErrSSHCommand
		}
	}
	if o.ProxyCommand != "" && !safePathPattern(o.ProxyCommand) {
		return nil, ErrSSHCommand
	}
	if command != UploadPackCommand(u) {
		return nil, ErrSSHUse
	}
	args := []string{
		o.Binary, "-F", "/dev/null",
		"-o", "StrictHostKeyChecking=yes",
		"-o", "UserKnownHostsFile=" + o.KnownHostsFile,
		"-o", "GlobalKnownHostsFile=/dev/null",
		"-o", "CheckHostIP=no",
		"-o", "UpdateHostKeys=no",
		"-o", "IdentitiesOnly=yes",
		"-o", "IdentityFile=" + o.KeyFile,
		"-o", "IdentityAgent=none",
		"-o", "BatchMode=yes",
		"-o", "PasswordAuthentication=no",
		"-o", "KbdInteractiveAuthentication=no",
		"-o", "ControlMaster=no",
		"-o", "ControlPath=none",
		"-o", "PermitLocalCommand=no",
		"-o", "ForwardAgent=no",
		"-o", "ForwardX11=no",
		"-o", "ClearAllForwardings=yes",
		"-o", sendEnvOption,
	}
	if o.ProxyCommand != "" {
		args = append(args, "-o", "ProxyCommand="+o.ProxyCommand+" %h %p")
	}
	args = append(args, "-p", strconv.Itoa(u.EffectivePort()))
	if u.User != "" {
		args = append(args, "-l", u.User)
	}
	return append(args, "--", u.Host, command), nil
}

func UploadPackCommand(u URL) string {
	return uploadPack + " '" + u.Path + "'"
}

func ParseSSHInvocation(u URL, args []string) (string, error) {
	if u.Transport != TransportSSH {
		return "", ErrNotSSH
	}
	rest := args
	if len(rest) >= 2 && rest[0] == "-o" && rest[1] == sendEnvOption {
		rest = rest[2:]
	}
	if len(rest) >= 2 && rest[0] == "-p" {
		if rest[1] != strconv.Itoa(u.EffectivePort()) {
			return "", ErrSSHUse
		}
		rest = rest[2:]
	}
	if len(rest) != 2 {
		return "", ErrSSHUse
	}
	target := u.Host
	if u.User != "" {
		target = u.User + "@" + u.Host
	}
	if rest[0] != target || rest[1] != UploadPackCommand(u) {
		return "", ErrSSHUse
	}
	return rest[1], nil
}

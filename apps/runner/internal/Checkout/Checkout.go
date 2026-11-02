package checkout

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Khaymah/dhow/internal/gitref"
	"github.com/Khaymah/dhow/internal/gitsource"
)

const (
	Command       = "__checkout"
	EnvURL        = "DHOW_CHECKOUT_URL"
	EnvRef        = "DHOW_CHECKOUT_REF"
	EnvSha        = "DHOW_CHECKOUT_SHA"
	EnvDepth      = "DHOW_CHECKOUT_DEPTH"
	EnvCredential = "DHOW_CHECKOUT_CREDENTIAL" //nolint:gosec // The name of a flag variable, not a credential.
	EnvProxy      = "HTTPS_PROXY"
	envGitURL     = "DHOW_GIT_URL"
	envMarkers    = "DHOW_GIT_MARKERS"

	CredentialFile = "git-credential" //nolint:gosec // The name of the secret file, not a credential.
	KnownHostsFile = "known_hosts"
	SecretsDir     = "/run/dhow/secrets"
	SSHHelper      = "/run/dhow/bin/git-ssh"
	AskpassHelper  = "/run/dhow/bin/git-askpass" //nolint:gosec // A helper path, not a credential.
	ConnectHelper  = "/run/dhow/bin/connect"
	GitBinary      = "/usr/bin/git"
	SSHBinary      = "/usr/bin/ssh"
	ScratchDir     = "/tmp/dhow-git"
	hostKeyMarker  = "host-key-mismatch"

	ExitOK              = 0
	ExitFailed          = 1
	ExitRefMoved        = 3
	ExitHostKeyMismatch = 4
	ExitDiskFull        = 5
)

var (
	ErrInput    = errors.New("the checkout settings are not valid")
	ErrRefMoved = errors.New("the commit is no longer reachable from the ref, and fetching it by id failed")
)

type Options struct {
	Getenv     func(string) string
	Stdout     io.Writer
	Stderr     io.Writer
	Dir        string
	Git        string
	SSHHelper  string
	Askpass    string
	ScratchDir string
}

type plan struct {
	url        gitsource.URL
	ref        string
	sha        string
	depth      int
	credential bool
}

func parsePlan(getenv func(string) string) (plan, error) {
	u, err := gitsource.Parse(getenv(EnvURL))
	if err != nil || u.String() != getenv(EnvURL) {
		return plan{}, fmt.Errorf("%w: the repository address", ErrInput)
	}
	ref := getenv(EnvRef)
	if _, err := gitref.Parse(ref); err != nil {
		return plan{}, fmt.Errorf("%w: the ref", ErrInput)
	}
	sha := getenv(EnvSha)
	if gitref.CheckSha(sha) != nil {
		return plan{}, fmt.Errorf("%w: the commit", ErrInput)
	}
	depth, err := strconv.Atoi(getenv(EnvDepth))
	if err != nil || depth < 1 || depth > gitsource.MaxDepth {
		return plan{}, fmt.Errorf("%w: the depth", ErrInput)
	}
	return plan{url: u, ref: ref, sha: sha, depth: depth, credential: getenv(EnvCredential) == "1"}, nil
}

func (o *Options) defaults() {
	if o.Git == "" {
		o.Git = GitBinary
	}
	if o.SSHHelper == "" {
		o.SSHHelper = SSHHelper
	}
	if o.Askpass == "" {
		o.Askpass = AskpassHelper
	}
	if o.ScratchDir == "" {
		o.ScratchDir = ScratchDir
	}
}

func Run(ctx context.Context, o Options) int {
	o.defaults()
	p, err := parsePlan(o.Getenv)
	if err != nil {
		_, _ = fmt.Fprintf(o.Stderr, "dhow: checkout: %v\n", err)
		return ExitFailed
	}
	r, err := newRepo(o, p)
	if err != nil {
		_, _ = fmt.Fprintf(o.Stderr, "dhow: checkout: %v\n", err)
		return ExitFailed
	}
	err = r.checkout(ctx)
	switch {
	case err == nil:
		_, _ = fmt.Fprintf(o.Stdout, "dhow: checked out %s at %s\n", p.ref, p.sha)
		return ExitOK
	case r.hostKeyMismatch():
		_, _ = fmt.Fprintln(o.Stderr, "dhow: HOST_KEY_MISMATCH: the server's host key is not the one pinned for this repository")
		return ExitHostKeyMismatch
	case r.diskFull:
		_, _ = fmt.Fprintln(o.Stderr, "dhow: CHECKOUT_DISK_FULL: the workspace ran out of space")
		return ExitDiskFull
	case errors.Is(err, ErrRefMoved):
		_, _ = fmt.Fprintf(o.Stderr, "dhow: CHECKOUT_REF_MOVED: %s no longer leads to %s\n", p.ref, p.sha)
		return ExitRefMoved
	default:
		_, _ = fmt.Fprintf(o.Stderr, "dhow: checkout: %v\n", err)
		return ExitFailed
	}
}

type repo struct {
	o        Options
	p        plan
	config   []string
	env      []string
	diskFull bool
	refGone  bool
}

func newRepo(o Options, p plan) (*repo, error) {
	home := filepath.Join(o.ScratchDir, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		return nil, fmt.Errorf("prepare the git home: %w", err)
	}
	config := gitsource.SafeConfig()
	eo := gitsource.EnvOptions{Home: home, HTTPSProxy: o.Getenv(EnvProxy)}
	if p.url.Transport == gitsource.TransportSSH {
		var err error
		if config, err = gitsource.CheckoutConfig(o.SSHHelper); err != nil {
			return nil, err
		}
	} else if p.credential {
		eo.Askpass = o.Askpass
	}
	env := append(gitsource.Env(eo), envGitURL+"="+p.url.String(), envMarkers+"="+o.ScratchDir)
	return &repo{o: o, p: p, config: config, env: env}, nil
}

func (r *repo) git(ctx context.Context, args []string, stdout io.Writer) error {
	cmd := exec.CommandContext(ctx, r.o.Git, args...) //nolint:gosec // gitsource builds every argument, with -- before the address and ref.
	cmd.Dir = r.o.Dir
	cmd.Env = r.env
	var errb bytes.Buffer
	cmd.Stdout = stdout
	cmd.Stderr = io.MultiWriter(r.o.Stderr, &errb)
	err := cmd.Run()
	if err != nil && strings.Contains(errb.String(), "No space left on device") {
		r.diskFull = true
	}
	if err != nil && strings.Contains(errb.String(), "couldn't find remote ref") {
		r.refGone = true
	}
	return err
}

func (r *repo) checkout(ctx context.Context) error {
	if err := r.git(ctx, gitsource.InitArgs(r.config), r.o.Stdout); err != nil {
		return fmt.Errorf("git init: %w", err)
	}
	fetch, err := gitsource.FetchArgsWith(r.config, r.p.url, r.p.ref, r.p.depth)
	if err != nil {
		return err
	}
	if err := r.git(ctx, fetch, r.o.Stdout); err != nil && !r.refGone {
		return fmt.Errorf("git fetch: %w", err)
	}
	if r.refGone || !r.reaches(ctx) {
		_, _ = fmt.Fprintf(r.o.Stdout, "dhow: %s moved away from %s, so the commit is fetched by its id\n", r.p.ref, r.p.sha)
		bySha, err := gitsource.FetchArgsWith(r.config, r.p.url, r.p.sha, r.p.depth)
		if err != nil {
			return err
		}
		if err := r.git(ctx, bySha, r.o.Stdout); err != nil || !r.reaches(ctx) {
			return ErrRefMoved
		}
	}
	co, err := gitsource.CheckoutArgs(r.config, r.p.sha)
	if err != nil {
		return err
	}
	if err := r.git(ctx, co, r.o.Stdout); err != nil {
		return fmt.Errorf("git checkout: %w", err)
	}
	return nil
}

func (r *repo) reaches(ctx context.Context) bool {
	var head bytes.Buffer
	if err := r.git(ctx, gitsource.FetchHeadArgs(r.config), &head); err != nil {
		return false
	}
	if strings.TrimSpace(head.String()) == r.p.sha {
		return true
	}
	anc, err := gitsource.IsAncestorArgs(r.config, r.p.sha)
	if err != nil {
		return false
	}
	return r.git(ctx, anc, io.Discard) == nil
}

func (r *repo) hostKeyMismatch() bool {
	_, err := os.Lstat(filepath.Join(r.o.ScratchDir, hostKeyMarker))
	return err == nil
}

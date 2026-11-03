package installer

import (
	"bytes"
	"context"
	"fmt"
	"net/netip"
	"regexp"
	"strings"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
)

const composeWaitSeconds = "180"

var releaseVersionRe = regexp.MustCompile(`^v?\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`)

func ComposeImageFor(version string) string {
	if releaseVersionRe.MatchString(version) {
		return ComposeImage + ":" + strings.TrimPrefix(version, "v")
	}
	return ComposeImage + ":latest"
}

func composeCmd(args ...string) Command {
	return Cmd("docker", append([]string{"compose", "--project-directory", ComposeDir, "--file", ComposeFilePath}, args...)...)
}

func composeUp() Command {
	return composeCmd("up", "--detach", "--wait", "--wait-timeout", composeWaitSeconds)
}

func (b *Builder) composeSteps() []Step {
	steps := []Step{
		DirStep(b.Sys, b.M.T(KeyStepConfigDir), ConfigDir, modeRootDir),
		DirStep(b.Sys, b.M.T(KeyStepComposeDir), ComposeDir, modeRootDir),
		FileStep(b.Sys, b.M.T(KeyStepComposeFile), ComposeFilePath, ComposeFile, modeRootPublic),
		DirStep(b.Sys, b.M.T(KeyStepSecretsDir), ComposeSecrets, modeRootDir),
		b.track(ContainerSecretFileStep(b.Sys, b.M.T(KeyStepKeyring), ComposeKeyring, generateKeyring)),
		b.composeEnvStep(),
		b.answersStep(),
		CommandStep(b.Sys, b.M.T(KeyStepComposeUp), nil, []Command{composeUp()}, nil),
	}
	if b.A.ManagedProxy() || (b.A.Proxy.Kind == ProxyExisting && b.A.Proxy.TrustedProxies == "") {
		steps = append(steps, b.gatewayStep())
	}
	return steps
}

func (b *Builder) composeTrusted(existing []EnvEntry) string {
	if b.A.Proxy.TrustedProxies != "" && !b.A.ManagedProxy() {
		return b.A.Proxy.TrustedProxies
	}
	return EnvValue(existing, config.EnvTrustedProxies)
}

func (b *Builder) renderComposeEnv(existing []EnvEntry) ([]byte, error) {
	key := EnvValue(existing, config.EnvSecretKey)
	if key == "" {
		k, err := RandomHex(SecretKeyBytes)
		if err != nil {
			return nil, err
		}
		key = k
	}
	pw := EnvValue(existing, envPGPass)
	if pw == "" {
		p, err := RandomHex(DBPasswordBytes)
		if err != nil {
			return nil, err
		}
		pw = p
	}
	return composeEnv(b.A, existing, key, pw, ComposeImageFor(b.Version), b.composeTrusted(existing)), nil
}

func (b *Builder) composeEnvStep() Step {
	var backup []byte
	return Step{
		Title: b.M.T(KeyStepComposeEnv),
		Files: []string{ComposeEnvPath},
		Done: func(context.Context) (bool, error) {
			current, err := b.Sys.ReadFile(ComposeEnvPath)
			if err != nil {
				return false, nil
			}
			want, err := b.renderComposeEnv(ParseEnv(current))
			if err != nil {
				return false, err
			}
			same, err := sameFile(b.Sys, ComposeEnvPath, want, modeRootPrivate.Perm)
			return same, err
		},
		Apply: func(context.Context) error {
			backup = b.readExisting(ComposeEnvPath)
			data, err := b.renderComposeEnv(ParseEnv(backup))
			if err != nil {
				return err
			}
			return b.Sys.WriteFile(ComposeEnvPath, data, modeRootPrivate)
		},
		Undo: func(context.Context) error {
			if backup != nil {
				return b.Sys.WriteFile(ComposeEnvPath, backup, modeRootPrivate)
			}
			return b.Sys.Remove(ComposeEnvPath)
		},
	}
}

func (b *Builder) gateway(ctx context.Context) (string, error) {
	out, err := run(ctx, b.Sys, Cmd("docker", "network", "inspect", ComposeEdgeNet, "--format", "{{(index .IPAM.Config 0).Gateway}}"))
	if err != nil {
		return "", err
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(string(out.Stdout)))
	if err != nil {
		return "", fmt.Errorf("docker network %s has no gateway address", ComposeEdgeNet)
	}
	return netip.PrefixFrom(addr, addr.BitLen()).String(), nil
}

func (b *Builder) gatewayStep() Step {
	var backup []byte
	return Step{
		Title:    b.M.T(KeyStepGateway),
		Files:    []string{ComposeEnvPath},
		Commands: []string{composeUp().String()},
		Done: func(ctx context.Context) (bool, error) {
			gw, err := b.gateway(ctx)
			if err != nil {
				return false, nil
			}
			return EnvValue(ParseEnv(b.readExisting(ComposeEnvPath)), config.EnvTrustedProxies) == gw, nil
		},
		Apply: func(ctx context.Context) error {
			gw, err := b.gateway(ctx)
			if err != nil {
				return err
			}
			backup = b.readExisting(ComposeEnvPath)
			data := RenderEnv(nil, setEnv(ParseEnv(backup), config.EnvTrustedProxies, gw))
			if bytes.Equal(data, backup) {
				return nil
			}
			if err := b.Sys.WriteFile(ComposeEnvPath, data, modeRootPrivate); err != nil {
				return err
			}
			_, err = run(ctx, b.Sys, composeUp())
			return err
		},
		Undo: func(context.Context) error {
			if backup == nil {
				return nil
			}
			return b.Sys.WriteFile(ComposeEnvPath, backup, modeRootPrivate)
		},
	}
}

func setEnv(entries []EnvEntry, key, value string) []EnvEntry {
	for i := range entries {
		if entries[i].Key == key {
			entries[i].Value = value
			return entries
		}
	}
	return append(entries, EnvEntry{Key: key, Value: value})
}

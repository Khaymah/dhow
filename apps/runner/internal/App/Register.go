package app

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"runtime"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	client "github.com/Khaymah/dhow/apps/runner/internal/Client"
	config "github.com/Khaymah/dhow/apps/runner/internal/Config"
	identity "github.com/Khaymah/dhow/apps/runner/internal/Identity"
	"github.com/Khaymah/dhow/internal/buildinfo"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const (
	stdinMarker   = "-"
	maxNameLength = 100
)

var (
	tokenPattern     = regexp.MustCompile(`^` + config.RegistrationTokenPrefix + `[A-Za-z0-9_-]{16,200}$`)
	ErrNoTokenFile   = errors.New("--token-file is required (a path, or - for stdin)")
	ErrTokenFormat   = errors.New("the registration token does not look like dhow_rrt_...; copy it again from the Runners page")
	ErrRegisterReply = errors.New("the server answered the registration with invalid data")
	ErrExtraArgs     = errors.New("unexpected arguments")
)

func runRegister(ctx context.Context, args []string, env Env) error {
	if err := refuseTokenFlags(args); err != nil {
		return err
	}
	fs := newFlags(CommandRegister, env.Stdio.Err)
	serverURL := fs.String("url", "", "the Dhow server address, for example https://dhow.example.com")
	tokenFile := fs.String("token-file", "", "a file that holds the registration token, or - to read it from stdin")
	name := fs.String("name", "", "the runner name shown in Dhow (default: the host name)")
	dev := fs.Bool("dev", false, "allow plain http:// to localhost, for local development only")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("%w: %s", ErrExtraArgs, strings.Join(fs.Args(), " "))
	}
	if *tokenFile == "" {
		return ErrNoTokenFile
	}
	if err := env.Harden(); err != nil {
		return err
	}
	machine, err := env.Machine()
	if err != nil {
		return err
	}
	cfg, err := config.Load(env.Getenv, config.Options{Dev: *dev, URL: *serverURL, Machine: machine})
	if err != nil {
		return err
	}
	if cfg.URL == nil {
		return fmt.Errorf("--url or %s: %w", config.EnvURL, config.ErrNeedsURL)
	}
	token, err := ReadToken(*tokenFile, env.Stdio.In)
	if err != nil {
		return err
	}
	id, err := register(ctx, cfg, token, RunnerName(*name))
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(env.Stdio.Out, "Registered runner %s in organization %s with labels [%s].\nStart it with: dhow-runner run\n",
		id.RunnerID, id.OrganizationID, strings.Join(id.Labels, ", "))
	return err
}

func runnerFeatures(cfg config.Config) []runnerv1.RunnerFeature {
	features := []runnerv1.RunnerFeature{runnerv1.RunnerFeature_RUNNER_FEATURE_PLUGINS, runnerv1.RunnerFeature_RUNNER_FEATURE_REGISTRY_AUTH}
	if cfg.CheckoutImage != "" {
		features = append(features, runnerv1.RunnerFeature_RUNNER_FEATURE_CHECKOUT,
			runnerv1.RunnerFeature_RUNNER_FEATURE_ARTIFACTS, runnerv1.RunnerFeature_RUNNER_FEATURE_CACHE)
		if caps := driverCapabilities(cfg); caps.Inputs && caps.ExactEgress {
			features = append(features, runnerv1.RunnerFeature_RUNNER_FEATURE_DEPLOY_TARGETS)
		}
	}
	return features
}

func register(ctx context.Context, cfg config.Config, token, name string) (*identity.Identity, error) {
	if err := config.EnsurePrivateDir(cfg.StateDir); err != nil {
		return nil, err
	}
	if exists, err := identity.Exists(cfg.SealedIdentityPath()); err != nil || exists {
		return nil, errors.Join(identity.ErrExists, err)
	}
	return RegisterWith(ctx, cfg, token, name, identity.NewFileStore(cfg.StateDir))
}

func RegisterWith(ctx context.Context, cfg config.Config, token, name string, store identity.Store) (*identity.Identity, error) {
	if exists, err := store.Exists(); err != nil || exists {
		return nil, errors.Join(identity.ErrExists, err)
	}
	keys, err := identity.GenerateKeys()
	if err != nil {
		return nil, err
	}
	c, err := client.New(client.Options{BaseURL: cfg.URL, CAFile: cfg.CAFile, Dev: cfg.Dev, Version: buildinfo.Get().Version})
	if err != nil {
		return nil, err
	}
	defer c.CloseIdle()
	res, err := c.Register(ctx, &runnerv1.RegisterRequest{
		Token: token, Name: name, Os: runtime.GOOS, Arch: runtime.GOARCH, Version: buildinfo.Get().Version,
		IsolationLevels: driverIsolation(cfg), Capacity: int32(cfg.Capacity), //nolint:gosec // Capacity is at most 16.
		SigningPublicKey: keys.SigningPublicKey(), EncryptionPublicKey: keys.EncryptionPublicKey(), Proof: keys.RegisterProof(cfg.Origin),
		Features: runnerFeatures(cfg),
	})
	if err != nil {
		return nil, fmt.Errorf("register with %s: %w", cfg.URL, err)
	}
	if _, err := uuid.Parse(res.GetRunnerId()); err != nil {
		return nil, ErrRegisterReply
	}
	if len(res.GetServerSigningPublicKey()) != ed25519.PublicKeySize || res.GetServerKeyVersion() < 1 {
		return nil, ErrRegisterReply
	}
	id := &identity.Identity{
		Keys: keys, RunnerID: res.GetRunnerId(), OrganizationID: res.GetOrganizationId(), ServerURL: cfg.URL.String(),
		ServerOrigin: cfg.Origin, ServerSigningKey: res.GetServerSigningPublicKey(), ServerKeyVersion: res.GetServerKeyVersion(),
		KeyVersion: identity.FirstKeyVersion, Labels: res.GetLabels(),
	}
	if err := store.Save(id); err != nil {
		return nil, fmt.Errorf("the server registered runner %s but its identity could not be saved, so revoke it and register again: %w",
			id.RunnerID, err)
	}
	return id, nil
}

func ReadToken(path string, stdin io.Reader) (string, error) {
	r := stdin
	if path != stdinMarker {
		f, err := os.Open(path) //nolint:gosec // The operator names the token file.
		if err != nil {
			return "", fmt.Errorf("read the token file: %w", err)
		}
		defer func() { _ = f.Close() }()
		r = f
	}
	line, err := bufio.NewReader(io.LimitReader(r, config.MaxRegistrationToken+2)).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read the token: %w", err)
	}
	token := strings.TrimSpace(line)
	if !tokenPattern.MatchString(token) {
		return "", ErrTokenFormat
	}
	return token, nil
}

func RunnerName(flagName string) string {
	name := strings.TrimSpace(flagName)
	if name == "" {
		host, err := os.Hostname()
		if err != nil || host == "" {
			host = "dhow-runner"
		}
		name = host
	}
	for utf8.RuneCountInString(name) > maxNameLength {
		_, size := utf8.DecodeLastRuneInString(name)
		name = name[:len(name)-size]
	}
	return name
}

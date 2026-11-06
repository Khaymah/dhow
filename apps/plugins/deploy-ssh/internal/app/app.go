package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"syscall"
	"time"

	"github.com/Khaymah/dhow/apps/plugins/deploy-ssh/internal/config"
	"github.com/Khaymah/dhow/internal/deployrelease"
	"github.com/Khaymah/dhow/internal/deployrelease/sshremote"
	"github.com/Khaymah/dhow/internal/jobspec"
	"github.com/Khaymah/dhow/internal/netguard"
)

const (
	maxKeyBytes    = 16 << 10
	proxyDialLimit = 15 * time.Second
	cancelGrace    = jobspec.DeployConnGrace
)

var codes = map[int]string{
	deployrelease.ExitFailed:          "DEPLOY_FAILED",
	deployrelease.ExitInput:           "PLUGIN_INPUT_INVALID",
	deployrelease.ExitRolledBack:      "DEPLOY_ROLLED_BACK",
	deployrelease.ExitHealthFailed:    "DEPLOY_HEALTH_FAILED",
	deployrelease.ExitRollbackFailed:  "DEPLOY_ROLLBACK_FAILED",
	deployrelease.ExitHostKeyMismatch: "HOST_KEY_MISMATCH",
	deployrelease.ExitUnsafeArtifact:  "ARTIFACT_UNSAFE",
	deployrelease.ExitTargetLocked:    "TARGET_LOCKED",
	deployrelease.ExitCurrentNotLink:  "CURRENT_NOT_A_SYMLINK",
	deployrelease.ExitConnect:         "TARGET_HOST_UNREACHABLE",
	deployrelease.ExitStateCorrupt:    "TARGET_STATE_CORRUPT",
	deployrelease.ExitCheckFailed:     "TARGET_CHECK_FAILED",
}

func Code(exit int) string {
	if c, ok := codes[exit]; ok {
		return c
	}
	return "DEPLOY_FAILED"
}

func readKey(path string) ([]byte, error) {
	f, err := os.Open(path) //nolint:gosec // The path is the key file the runner mounted for this step only.
	if err != nil {
		return nil, fmt.Errorf("open the key file: %w", err)
	}
	defer func() { _ = f.Close() }()
	raw, err := io.ReadAll(io.LimitReader(f, maxKeyBytes+1))
	if err != nil || len(raw) == 0 || len(raw) > maxKeyBytes {
		return nil, errors.New("the key file is empty, unreadable or too large")
	}
	return raw, nil
}

func openReport(path string) (io.WriteCloser, error) {
	if path == "" {
		return nil, nil
	}
	return os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE|syscall.O_NOFOLLOW, 0o600) //nolint:gosec // The path is the report file the runner named for this step.
}

func network(s config.Settings) (sshremote.DialFunc, http.RoundTripper, error) {
	if s.ProxyURL != "" {
		dial, err := sshremote.ProxyDialer(s.ProxyURL, proxyDialLimit)
		if err != nil {
			return nil, nil, err
		}
		proxy, err := url.Parse(s.ProxyURL)
		if err != nil {
			return nil, nil, err
		}
		return dial, &http.Transport{Proxy: http.ProxyURL(proxy), TLSHandshakeTimeout: deployrelease.MaxHealthTime}, nil
	}
	guard := netguard.Dialer{Policy: netguard.Policy{Allowed: s.AllowPrivate}}
	return guard.DialContext, &http.Transport{DialContext: guard.DialContext, TLSHandshakeTimeout: deployrelease.MaxHealthTime}, nil
}

func Run(ctx context.Context, s config.Settings, log *slog.Logger) int {
	key, err := readKey(s.KeyFile)
	if err != nil {
		log.Error("the deploy key cannot be used", "error", err.Error())
		return deployrelease.ExitInput
	}
	dial, transport, err := network(s)
	if err != nil {
		log.Error("the network settings are not valid", "error", err.Error())
		return deployrelease.ExitInput
	}
	var reporter deployrelease.Reporter = deployrelease.DiscardReporter{}
	if out, err := openReport(s.ReportPath); err != nil {
		log.Warn("the deploy report cannot be opened, so the deployment is not reported", "error", err.Error())
	} else if out != nil {
		defer func() { _ = out.Close() }()
		reporter = deployrelease.NewStreamReporter(out)
		if s.Deploy.Rollback {
			reporter = deployrelease.NewRetagReporter(reporter, s.Deploy.ReleaseID, s.ReportID)
		}
	}
	if s.Mode == jobspec.TargetModeCheck {
		return runCheck(ctx, s, log, reporter, key, dial)
	}
	fleet := &deployrelease.Fleet{
		Config: s.Deploy, BatchSize: s.BatchSize, BatchPause: s.BatchPause,
		Doer: deployrelease.NewHealthClient(transport), Report: reporter, Log: log, Clock: deployrelease.RealClock(),
	}
	for _, h := range s.Step.Hosts {
		fleet.Hosts = append(fleet.Hosts, deployrelease.Host{ID: h.HostID, Name: h.Host, Connect: connector(s, h, key, dial)})
	}
	f := fleet.Run(ctx)
	if f == nil {
		log.Info("the release is live", "release", s.Deploy.ReleaseID, "servers", len(s.Step.Hosts))
		return deployrelease.ExitOK
	}
	log.Error("deployment failed", "code", Code(f.Exit), "reason", f.Reason, "detail", f.Text)
	return f.Exit
}

func connector(s config.Settings, host jobspec.TargetHost, key []byte, dial sshremote.DialFunc) func(context.Context) (deployrelease.Remote, error) {
	return func(ctx context.Context) (deployrelease.Remote, error) {
		life, end := context.WithCancel(context.WithoutCancel(ctx))
		context.AfterFunc(ctx, func() { time.AfterFunc(deployrelease.RollbackBudget+cancelGrace, end) })
		c, err := sshremote.Connect(life, sshremote.Options{
			Host: host.Host, Port: host.Port, User: s.Step.User, PrivateKey: key, KnownHostsLine: host.KnownHostsLine, Dial: dial,
		})
		if err != nil {
			end()
			return nil, err
		}
		return c, nil
	}
}

func runCheck(ctx context.Context, s config.Settings, log *slog.Logger, reporter deployrelease.Reporter, key []byte, dial sshremote.DialFunc) int {
	checker := &deployrelease.Checker{DeployPath: s.Step.DeployPath, Report: reporter, Log: log, Clock: deployrelease.RealClock()}
	for _, h := range s.Step.Hosts {
		checker.Hosts = append(checker.Hosts, deployrelease.Host{ID: h.HostID, Name: h.Host, Connect: connector(s, h, key, dial)})
	}
	f := checker.Run(ctx)
	if f == nil {
		log.Info("every server of the target is ready", "servers", len(s.Step.Hosts))
		return deployrelease.ExitOK
	}
	log.Error("the check found a problem", "code", Code(f.Exit), "detail", f.Text)
	return f.Exit
}

package stack

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Khaymah/dhow/apps/plugins/deploy-compose/internal/policy"
	"github.com/Khaymah/dhow/internal/archive"
	"github.com/Khaymah/dhow/internal/deployrelease"
)

var ErrMissingFile = errors.New("the compose file is not in the artifact")

func (a *Activator) Inspect(ctx context.Context, cfg deployrelease.Config) error {
	f, err := os.Open(cfg.ArchivePath)
	if err != nil {
		return &deployrelease.Failure{Exit: deployrelease.ExitInput, Reason: deployrelease.CauseOther, Text: "the artifact cannot be opened", Err: err}
	}
	defer func() { _ = f.Close() }()
	var raw []byte
	var found bool
	var links []string
	hash := sha256.New()
	visit := func(e archive.Entry, body io.Reader) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		switch {
		case e.Kind == archive.KindSymlink:
			links = append(links, e.Name)
		case e.Name == a.File && e.Kind == archive.KindFile:
			found = true
			var readErr error
			raw, readErr = io.ReadAll(io.LimitReader(body, policy.MaxFileBytes+1))
			return readErr
		}
		return nil
	}
	if _, err := archive.Walk(io.TeeReader(f, hash), cfg.Limits, visit); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return &deployrelease.Failure{Exit: deployrelease.ExitUnsafeArtifact, Reason: deployrelease.CauseOther, Text: "the artifact is not safe to put on a server", Err: err}
	}
	if _, err := io.Copy(hash, f); err != nil {
		return &deployrelease.Failure{Exit: deployrelease.ExitInput, Reason: deployrelease.CauseOther, Text: "the artifact cannot be read", Err: err}
	}
	if cfg.ArchiveDigest != "" && hex.EncodeToString(hash.Sum(nil)) != cfg.ArchiveDigest {
		return &deployrelease.Failure{Exit: deployrelease.ExitUnsafeArtifact, Reason: deployrelease.CauseOther, Text: "the artifact is not the one whose digest was handed over", Err: deployrelease.ErrDigest}
	}
	if !found {
		return refusal(&policy.Violation{Code: policy.CodeInvalid, Detail: fmt.Sprint(ErrMissingFile)})
	}
	opt := a.Policy
	opt.DeployPath = cfg.DeployPath
	opt.Links = links
	if _, err := policy.Check(raw, opt); err != nil {
		return refusal(err)
	}
	a.links = links
	return nil
}

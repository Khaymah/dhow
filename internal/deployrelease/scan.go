package deployrelease

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Khaymah/dhow/internal/archive"
	"github.com/Khaymah/dhow/internal/jobspec"
)

type scan struct {
	Entries int
	Bytes   int64
}

func validID(id string) bool { return jobspec.ValidReleaseID(id) }

func (d *run) scanArchive() (scan, *Failure) {
	f, err := os.Open(d.cfg.ArchivePath)
	if err != nil {
		return scan{}, fail(ExitInput, CauseOther, "the artifact cannot be opened", err)
	}
	defer func() { _ = f.Close() }()
	hash := sha256.New()
	totals, err := archive.Walk(io.TeeReader(f, hash), d.cfg.Limits, func(archive.Entry, io.Reader) error { return nil })
	if err != nil {
		return scan{}, archiveFailure(err)
	}
	if _, err := io.Copy(hash, f); err != nil {
		return scan{}, fail(ExitInput, CauseOther, "the artifact cannot be read", err)
	}
	if d.cfg.ArchiveDigest != "" && hex.EncodeToString(hash.Sum(nil)) != d.cfg.ArchiveDigest {
		return scan{}, fail(ExitUnsafeArtifact, CauseOther, "the artifact does not match its digest", ErrDigest)
	}
	return scan{Entries: totals.Entries, Bytes: totals.Bytes}, nil
}

func archiveFailure(err error) *Failure {
	switch {
	case errors.Is(err, archive.ErrUnsafe), errors.Is(err, archive.ErrTooLarge):
		return fail(ExitUnsafeArtifact, CauseOther, "the artifact has an unsafe entry", fmt.Errorf("%w: %w", ErrUnsafeArtifact, err))
	default:
		return fail(ExitFailed, CauseUpload, "the artifact cannot be read", err)
	}
}

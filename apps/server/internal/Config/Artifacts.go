package config

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	EnvBlobDir                 = "DHOW_BLOB_DIR"
	EnvArtifactMaxBytes        = "DHOW_ARTIFACT_MAX_BYTES"
	EnvArtifactRetentionDays   = "DHOW_ARTIFACT_RETENTION_DAYS"
	EnvCacheMaxBytesPerProject = "DHOW_CACHE_MAX_BYTES_PER_PROJECT"
	EnvArtifactMaxBytesPerOrg  = "DHOW_ARTIFACT_MAX_BYTES_PER_ORG"
)

const (
	BlobDirDefault               = "/var/lib/dhow/blobs"
	BlobDirDev                   = "dhow-blobs"
	ArtifactMaxBytesDefault      = 1 << 30
	ArtifactMaxBytesMin          = 1 << 20
	ArtifactMaxBytesMax          = 64 << 30
	ArtifactRetentionDefault     = 30
	ArtifactRetentionMax         = 365
	CacheMaxBytesDefault         = 5 << 30
	CacheMaxBytesMin             = 16 << 20
	CacheMaxBytesMax             = 1 << 40
	ArtifactOrgMaxBytesDefault   = 50 << 30
	ArtifactOrgMaxBytesMin       = 16 << 20
	ArtifactOrgMaxBytesMax       = 1 << 50
	ArtifactForkShareDivisor     = 10
	BlobFreeSpaceReserve         = 512 << 20
	BlobSpaceCheckEvery          = 64 << 20
	BlobsPath                    = "/blobs/"
	BlobLeaseHeader              = "Dhow-Lease-Id"
	BlobFencingHeader            = "Dhow-Fencing-Token"
	BlobDownloadTokenHeader      = "Dhow-Download-Token"
	BlobKindHeader               = "Dhow-Blob-Kind"
	BlobNameHeader               = "Dhow-Blob-Name"
	BlobKindArtifact             = "artifact"
	BlobKindCache                = "cache"
	BlobIdleTimeout              = time.Minute
	BlobSweepEvery               = 10 * time.Minute
	BlobSweepGrace               = 6 * time.Hour
	BlobTempGrace                = 24 * time.Hour
	BlobSweepBatch               = 500
	BlobSweepLockKey             = int64(0x64686f772d626c62)
	ArtifactGrantKeyLabel        = "dhow/artifact-grant/v1"
	ArtifactInputGrantTTL        = 72 * time.Hour
	CacheGrantTTL                = 15 * time.Minute
	ArtifactListMax              = 256
	CacheKeyMaxBytes             = 512
	CacheScopeMaxBytes           = 300
	LogScrubEvery                = time.Hour
	LogScrubBatch                = 1000
	ArtifactDownloadNameFallback = "artifact"
)

type Artifacts struct {
	BlobDir       string
	BlobDirIsDev  bool
	MaxBytes      int64
	RetentionDays int
	CacheMaxBytes int64
	OrgMaxBytes   int64
}

func DefaultArtifacts() Artifacts {
	return Artifacts{
		BlobDir: BlobDirDefault, MaxBytes: ArtifactMaxBytesDefault, RetentionDays: ArtifactRetentionDefault,
		CacheMaxBytes: CacheMaxBytesDefault, OrgMaxBytes: ArtifactOrgMaxBytesDefault,
	}
}

func (a Artifacts) ForkMaxBytes() int64 { return a.OrgMaxBytes / ArtifactForkShareDivisor }

func (a Artifacts) Retention() time.Duration { return time.Duration(a.RetentionDays) * 24 * time.Hour }

func boundedBytes(name, v string, def, lowest, highest int64) (int64, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return def, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < lowest || n > highest {
		return 0, errors.New(name + ": must be a whole number of bytes from " + strconv.FormatInt(lowest, 10) + " to " +
			strconv.FormatInt(highest, 10))
	}
	return n, nil
}

func loadArtifacts(getenv func(string) string, dev bool) (Artifacts, []error) {
	out := DefaultArtifacts()
	var errs []error
	switch dir := strings.TrimSpace(getenv(EnvBlobDir)); {
	case dir != "" && !filepath.IsAbs(dir):
		errs = append(errs, errors.New(EnvBlobDir+": must be an absolute path"))
	case dir != "":
		out.BlobDir = filepath.Clean(dir)
	case dev:
		out.BlobDir, out.BlobDirIsDev = filepath.Join(os.TempDir(), BlobDirDev+"-"+strconv.Itoa(os.Getuid())), true
	}
	var err error
	if out.MaxBytes, err = boundedBytes(EnvArtifactMaxBytes, getenv(EnvArtifactMaxBytes), ArtifactMaxBytesDefault,
		ArtifactMaxBytesMin, ArtifactMaxBytesMax); err != nil {
		errs = append(errs, err)
	}
	if out.CacheMaxBytes, err = boundedBytes(EnvCacheMaxBytesPerProject, getenv(EnvCacheMaxBytesPerProject), CacheMaxBytesDefault,
		CacheMaxBytesMin, CacheMaxBytesMax); err != nil {
		errs = append(errs, err)
	}
	if out.OrgMaxBytes, err = boundedBytes(EnvArtifactMaxBytesPerOrg, getenv(EnvArtifactMaxBytesPerOrg), ArtifactOrgMaxBytesDefault,
		ArtifactOrgMaxBytesMin, ArtifactOrgMaxBytesMax); err != nil {
		errs = append(errs, err)
	}
	days, err := boundedSmall(getenv(EnvArtifactRetentionDays), ArtifactRetentionDefault, 1, ArtifactRetentionMax)
	if err != nil {
		errs = append(errs, errors.New(EnvArtifactRetentionDays+": "+err.Error()))
	}
	out.RetentionDays = days
	return out, errs
}

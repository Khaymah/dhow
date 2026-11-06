package artifacts

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/encoding/protojson"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	artifactindex "github.com/Khaymah/dhow/apps/server/internal/Core/ArtifactIndex"
	blob "github.com/Khaymah/dhow/apps/server/internal/Core/Blob"
	appcrypto "github.com/Khaymah/dhow/apps/server/internal/Core/Crypto"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

var ErrNoStore = errors.New("the artifacts service needs a blob store")

type Options struct {
	DB        *pgxpool.Pool
	Store     blob.Store
	Config    config.Artifacts
	SecretKey []byte
	Log       *slog.Logger
	Now       func() time.Time
}

type Service struct {
	db     *pgxpool.Pool
	store  blob.Store
	cfg    config.Artifacts
	grants grants
	log    *slog.Logger
	now    func() time.Time
}

var _ artifactindex.Index = (*Service)(nil)

func NewService(o Options) (*Service, error) {
	if o.Store == nil {
		return nil, ErrNoStore
	}
	key, err := appcrypto.DeriveKey(o.SecretKey, config.ArtifactGrantKeyLabel)
	if err != nil {
		return nil, err
	}
	now := o.Now
	if now == nil {
		now = time.Now
	}
	cfg := o.Config
	if cfg.MaxBytes <= 0 {
		cfg.MaxBytes = config.ArtifactMaxBytesDefault
	}
	if cfg.CacheMaxBytes <= 0 {
		cfg.CacheMaxBytes = config.CacheMaxBytesDefault
	}
	if cfg.OrgMaxBytes <= 0 {
		cfg.OrgMaxBytes = config.ArtifactOrgMaxBytesDefault
	}
	if cfg.RetentionDays <= 0 {
		cfg.RetentionDays = config.ArtifactRetentionDefault
	}
	return &Service{db: o.DB, store: o.Store, cfg: cfg, grants: grants{key: key}, log: o.Log, now: now}, nil
}

func (s *Service) queries() *sqlc.Queries { return sqlc.New(s.db) }

func decodeSpec(raw []byte) (*runnerv1.JobSpec, error) {
	var spec runnerv1.JobSpec
	if err := (protojson.UnmarshalOptions{DiscardUnknown: false}).Unmarshal(raw, &spec); err != nil {
		return nil, fmt.Errorf("decode stored job spec: %w", err)
	}
	return &spec, nil
}

func (s *Service) Prepare(ctx context.Context, tx *sqlc.Queries, r artifactindex.Request) (artifactindex.Prepared, error) {
	out := artifactindex.Prepared{ArtifactMaxBytes: s.cfg.MaxBytes, CacheMaxBytes: s.cfg.CacheMaxBytes}
	if len(r.Needs) == 0 {
		return out, nil
	}
	rows, err := tx.ListRunJobArtifacts(ctx, sqlc.ListRunJobArtifactsParams{
		OrganizationID: r.OrganizationID, RunID: r.RunID, RunJobKeys: r.Needs,
	})
	if err != nil {
		return out, err
	}
	expires := s.now().Add(config.ArtifactInputGrantTTL)
	for _, a := range rows {
		d, ok := blob.DigestOf(a.Digest)
		if !ok {
			return out, fmt.Errorf("artifact %s has a malformed digest", a.Id)
		}
		if a.Untrusted && r.FeedsDeploy {
			return artifactindex.Prepared{Refused: artifactindex.ReasonUntrustedForDeploy}, nil
		}
		out.Inputs = append(out.Inputs, artifactindex.Input{
			Name: a.Name, Digest: d.String(), SizeBytes: a.SizeBytes, Untrusted: a.Untrusted,
			DownloadToken: s.grants.issue(grantInput, r.RunJobID, d, expires),
		})
	}
	return out, nil
}

func (s *Service) PrepareDigest(ctx context.Context, tx *sqlc.Queries, r artifactindex.DigestRequest) (artifactindex.Input, bool, error) {
	a, err := tx.FindRollbackArtifact(ctx, sqlc.FindRollbackArtifactParams{
		OrganizationID: r.OrganizationID, ProjectID: r.ProjectID, Digest: r.Digest, RollbackGraceSeconds: config.RollbackArtifactMaxKeep.Seconds(),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return artifactindex.Input{}, false, nil
	}
	if err != nil {
		return artifactindex.Input{}, false, err
	}
	d, ok := blob.DigestOf(a.Digest)
	if !ok {
		return artifactindex.Input{}, false, fmt.Errorf("artifact %s has a malformed digest", a.Name)
	}
	expires := s.now().Add(config.ArtifactInputGrantTTL)
	return artifactindex.Input{
		Name: a.Name, Digest: d.String(), SizeBytes: a.SizeBytes, DownloadToken: s.grants.issue(grantInput, r.RunJobID, d, expires),
	}, true, nil
}

func (s *Service) SeedBlob(ctx context.Context, d blob.Digest, body io.Reader, size int64) error {
	pending, err := s.store.Stage(ctx, blob.Put{Digest: d, Size: size, MaxBytes: max(size, 1), Body: body})
	if err != nil {
		return err
	}
	defer pending.Abort()
	stored, err := pending.Commit()
	if err != nil {
		return err
	}
	_, err = s.queries().UpsertBlob(ctx, sqlc.UpsertBlobParams{Digest: d[:], SizeBytes: stored.Size, StorageKey: stored.Key})
	return err
}

func (s *Service) StoreWithoutRecord(ctx context.Context, data []byte) error {
	d := blob.Digest(sha256.Sum256(data))
	pending, err := s.store.Stage(ctx, blob.Put{Digest: d, Size: int64(len(data)), MaxBytes: max(int64(len(data)), 1), Body: bytes.NewReader(data)})
	if err != nil {
		return err
	}
	_, err = pending.Commit()
	return err
}

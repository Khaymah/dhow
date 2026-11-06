package ingest

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	blob "github.com/Khaymah/dhow/apps/server/internal/Core/Blob"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	triggers "github.com/Khaymah/dhow/apps/server/internal/Core/Triggers"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Ingest/Models"
	"github.com/Khaymah/dhow/internal/archive"
)

type receivedJSON struct {
	Name      string     `json:"Name"`
	Digest    string     `json:"Digest"`
	SizeBytes int64      `json:"SizeBytes"`
	Created   bool       `json:"Created"`
	Triggered bool       `json:"Triggered"`
	Reason    string     `json:"Reason,omitempty"`
	RunID     *uuid.UUID `json:"RunId,omitempty"`
	RunNumber *int64     `json:"RunNumber,omitempty"`
	RunURL    string     `json:"RunUrl,omitempty"`
	Ref       string     `json:"Ref,omitempty"`
	Sha       string     `json:"Sha,omitempty"`
}

func hexOf(b []byte) string { return hex.EncodeToString(b) }

func receivedView(r models.Received) receivedJSON {
	out := receivedJSON{
		Name: r.Name, Digest: r.Digest, SizeBytes: r.SizeBytes, Created: r.Created, Triggered: r.Triggered, Reason: r.Reason, RunURL: r.RunURL,
	}
	if r.Run != nil {
		out.RunID, out.RunNumber = &r.Run.ID, &r.Run.Number
	}
	return out
}

func (s *Service) archiveLimits() archive.Limits {
	return archive.Limits{MaxEntries: archive.MaxEntries, MaxBytes: s.cfg.IngestMaxBytes * config.IngestExpandFactor}
}

func (s *Service) receive(
	ctx context.Context, w http.ResponseWriter, r *http.Request, tok tokenAuth, m Meta, d blob.Digest,
) (models.Received, error) {
	check := startArchiveCheck(s.archiveLimits())
	rc := http.NewResponseController(w)
	body := idleReader{r: http.MaxBytesReader(w, r.Body, s.cfg.IngestMaxBytes), rc: rc, idle: config.IngestIdleTimeout}
	pending, stageErr := s.store.Stage(ctx, blob.Put{
		Digest: d, Size: r.ContentLength, MaxBytes: s.cfg.IngestMaxBytes, Body: check.tee(body),
	})
	archiveErr := check.finish()
	if stageErr == nil {
		defer pending.Abort()
	}
	switch {
	case unsafeArchive(archiveErr):
		return models.Received{}, apperrors.ArchiveUnsafe()
	case stageErr != nil:
		return models.Received{}, stageFailure(stageErr)
	case archiveErr != nil:
		return models.Received{}, archiveErr
	}
	return s.record(ctx, pending, tok, m, d, r.ContentLength)
}

func stageFailure(err error) error {
	var tooBig *http.MaxBytesError
	switch {
	case errors.Is(err, blob.ErrTooLarge), errors.As(err, &tooBig):
		return apperrors.ArtifactTooLarge()
	case errors.Is(err, blob.ErrDigestMismatch), errors.Is(err, blob.ErrSizeMismatch):
		return apperrors.DigestMismatch()
	case errors.Is(err, blob.ErrNoSpace):
		return apperrors.StorageFull()
	default:
		return err
	}
}

func (s *Service) record(
	ctx context.Context, pending blob.Pending, tok tokenAuth, m Meta, d blob.Digest, size int64,
) (models.Received, error) {
	commitCtx := context.WithoutCancel(ctx)
	var out models.Received
	err := dbtx.Run(commitCtx, s.db, func(q *sqlc.Queries) error {
		org, project := tok.row.OrganizationId, tok.row.ProjectId
		if err := q.LockOrganizationArtifacts(commitCtx, org.String()); err != nil {
			return err
		}
		if err := q.LockIngestProject(commitCtx, project.String()); err != nil {
			return err
		}
		known, found, err := s.known(commitCtx, q, tok, m, d)
		if err != nil || found {
			out = known
			return err
		}
		if err := s.checkQuota(commitCtx, q, org, size); err != nil {
			return err
		}
		if err := q.LockBlobsShared(commitCtx, config.BlobSweepLockKey); err != nil {
			return err
		}
		row, err := s.insertArtifact(commitCtx, q, tok, m, d, size)
		if err != nil {
			return err
		}
		triggered, err := s.trigger(commitCtx, q, tok, m, row)
		if err != nil {
			return err
		}
		if _, err := pending.Commit(); err != nil {
			return fmt.Errorf("place the ingested blob: %w", err)
		}
		out, err = s.resultOf(commitCtx, q, row, true)
		if err != nil {
			return err
		}
		out.Reason = triggered
		return s.auditStored(commitCtx, q, tok, row, out)
	})
	return out, err
}

func (s *Service) insertArtifact(ctx context.Context, q *sqlc.Queries, tok tokenAuth, m Meta, d blob.Digest, size int64) (sqlc.IngestedArtifact, error) {
	if _, err := q.UpsertBlob(ctx, sqlc.UpsertBlobParams{Digest: d[:], SizeBytes: size, StorageKey: d.Key()}); err != nil {
		return sqlc.IngestedArtifact{}, err
	}
	if err := q.UpsertBlobOwner(ctx, sqlc.UpsertBlobOwnerParams{OrganizationID: tok.row.OrganizationId, Digest: d[:]}); err != nil {
		return sqlc.IngestedArtifact{}, err
	}
	prov, err := json.Marshal(m.Provenance)
	if err != nil {
		return sqlc.IngestedArtifact{}, fmt.Errorf("encode the provenance of an ingested artifact: %w", err)
	}
	return q.InsertIngestedArtifact(ctx, sqlc.InsertIngestedArtifactParams{
		OrganizationID: tok.row.OrganizationId, ProjectID: tok.row.ProjectId, TokenID: tok.row.Id, Name: m.Name, Digest: d[:], SizeBytes: size,
		Ref: m.Ref, RefKind: m.RefKind, Sha: m.Sha, Provenance: prov, ExpiresAt: s.now().Add(time.Duration(s.cfg.RetentionDays) * 24 * time.Hour),
	})
}

func actorOf(tok tokenAuth, m Meta) string {
	actor := "ingest:" + tok.row.Name
	if who := m.Provenance["Actor"]; who != "" {
		actor += " (" + who + ")"
	}
	return actor
}

func (s *Service) trigger(ctx context.Context, q *sqlc.Queries, tok tokenAuth, m Meta, row sqlc.IngestedArtifact) (string, error) {
	if !m.Trigger || s.triggers == nil {
		return models.ReasonTriggerDisabled, nil
	}
	got, err := s.triggers.Deliver(ctx, q, triggers.Trigger{
		OrganizationID: tok.row.OrganizationId, ProjectID: tok.row.ProjectId, DeliveryID: uuid.New(), Provider: triggers.ProviderIngest,
		Event: triggers.EventArtifact, Ref: m.Ref, Sha: m.Sha, Actor: actorOf(tok, m),
		Artifacts: []triggers.IngestedRef{{ID: row.Id, Name: row.Name}},
	})
	if err != nil {
		return "", err
	}
	return got.Reason, nil
}

func (s *Service) auditStored(ctx context.Context, q *sqlc.Queries, tok tokenAuth, row sqlc.IngestedArtifact, out models.Received) error {
	data := map[string]string{
		"Name": row.Name, "Digest": hexOf(row.Digest), "SizeBytes": strconv.FormatInt(row.SizeBytes, 10), "Ref": row.Ref, "Sha": row.Sha,
		"ProjectId": row.ProjectId.String(), "Uploader": tok.row.Name, "UploaderId": tok.row.Id.String(), "Triggered": strconv.FormatBool(out.Triggered),
	}
	if out.Run != nil {
		data["RunNumber"] = strconv.FormatInt(out.Run.Number, 10)
	}
	return s.audit(ctx, auditEntry{
		name: eventbus.ArtifactIngested, org: row.OrganizationId, target: targetArtifact, targetID: row.Id, data: data, tx: q,
	})
}

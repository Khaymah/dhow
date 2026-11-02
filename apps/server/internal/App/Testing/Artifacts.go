package apptest

import (
	"bytes"
	"context"
	"crypto/sha256"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type SeededArtifact struct {
	ID      uuid.UUID
	RunID   uuid.UUID
	Content []byte
	Digest  [32]byte
}

func (e *Env) SeedArtifact(project uuid.UUID, name string, content []byte, untrusted bool) SeededArtifact {
	e.T.Helper()
	ctx := context.Background()
	digest := sha256.Sum256(content)
	require.NoError(e.T, e.Wired.Artifacts.SeedBlob(ctx, digest, bytes.NewReader(content), int64(len(content))))
	run := e.SeedRun(project, "pending")
	var org, runJob uuid.UUID
	require.NoError(e.T, e.Pool.QueryRow(ctx,
		`INSERT INTO "RunJobs" ("OrganizationId", "RunId", "Key", "JobName", "JobId", "Position")
		 SELECT "OrganizationId", "Id", 'build', 'build', 'build', 0 FROM "PipelineRuns" WHERE "Id" = $1
		 ON CONFLICT ("RunId", "Key") DO UPDATE SET "UpdatedAt" = now()
		 RETURNING "OrganizationId", "Id"`, run).Scan(&org, &runJob))
	var id uuid.UUID
	require.NoError(e.T, e.Pool.QueryRow(ctx,
		`INSERT INTO "Artifacts" ("OrganizationId", "ProjectId", "RunId", "RunJobId", "RunJobKey", "JobId", "Name", "Digest",
		 "SizeBytes", "Untrusted", "ExpiresAt")
		 VALUES ($1, $2, $3, $4, 'build', $5, $6, $7, $8, $9, now() + interval '30 days') RETURNING "Id"`,
		org, project, run, runJob, uuid.New(), name, digest[:], len(content), untrusted).Scan(&id))
	return SeededArtifact{ID: id, RunID: run, Content: content, Digest: digest}
}

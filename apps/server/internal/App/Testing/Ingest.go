package apptest

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	blob "github.com/Khaymah/dhow/apps/server/internal/Core/Blob"
)

const IngestTokenPrefix = "dhow_ing_"

type IngestToken struct {
	ID    uuid.UUID
	Value string
}

type IngestSeed struct {
	Name      string
	Patterns  string
	ExpiresIn time.Duration
	Revoked   bool
}

func (e *Env) SeedIngestToken(project, creator uuid.UUID, seed IngestSeed) IngestToken {
	e.T.Helper()
	if seed.Name == "" {
		seed.Name = "ci-" + uuid.NewString()[:8]
	}
	if seed.Patterns == "" {
		seed.Patterns = `[{"Kind":"branch","Pattern":"main"},{"Kind":"tag","Pattern":"v*"}]`
	}
	if seed.ExpiresIn == 0 {
		seed.ExpiresIn = 90 * 24 * time.Hour
	}
	raw := make([]byte, 32)
	_, err := rand.Read(raw)
	require.NoError(e.T, err)
	body := base64.RawURLEncoding.EncodeToString(raw)
	value := IngestTokenPrefix + body
	sum := sha256.Sum256([]byte(value))
	var org, id uuid.UUID
	require.NoError(e.T, e.Pool.QueryRow(context.Background(), `SELECT "OrganizationId" FROM "Projects" WHERE "Id" = $1`, project).Scan(&org))
	now := e.Clock.Now()
	require.NoError(e.T, e.Pool.QueryRow(context.Background(),
		`INSERT INTO "IngestTokens" ("OrganizationId", "ProjectId", "Name", "TokenHash", "Prefix", "RefPatterns", "ExpiresAt", "CreatedByUserId")
		 VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8) RETURNING "Id"`,
		org, project, seed.Name, sum[:], IngestTokenPrefix+body[:8], seed.Patterns, now.Add(seed.ExpiresIn), creator).Scan(&id))
	if seed.Revoked {
		_, err := e.Pool.Exec(context.Background(), `UPDATE "IngestTokens" SET "RevokedAt" = now(), "RevokedByUserId" = $2 WHERE "Id" = $1`, id, creator)
		require.NoError(e.T, err)
	}
	return IngestToken{ID: id, Value: value}
}

func (e *Env) ExpireIngestToken(id uuid.UUID) {
	e.T.Helper()
	_, err := e.Pool.Exec(context.Background(), `UPDATE "IngestTokens" SET "ExpiresAt" = now() - interval '1 minute' WHERE "Id" = $1`, id)
	require.NoError(e.T, err)
}

type IngestUpload struct {
	Token      string
	Digest     string
	Body       []byte
	Name       string
	Ref        string
	RefKind    string
	Sha        string
	NoTrigger  bool
	Provenance string
	Headers    map[string]string
	Chunked    bool
}

func (u IngestUpload) headers() map[string]string {
	h := map[string]string{"Content-Type": "application/gzip"}
	if u.Token != "" {
		h["Authorization"] = "Bearer " + u.Token
	}
	set := func(k, v string) {
		if v != "" {
			h[k] = v
		}
	}
	set("Dhow-Ingest-Name", u.Name)
	set("Dhow-Ingest-Ref", u.Ref)
	set("Dhow-Ingest-Ref-Kind", u.RefKind)
	set("Dhow-Ingest-Sha", u.Sha)
	set("Dhow-Ingest-Provenance", u.Provenance)
	if u.NoTrigger {
		h["Dhow-Ingest-Trigger"] = "false"
	}
	for k, v := range u.Headers {
		h[k] = v
	}
	return h
}

func (e *Env) IngestPut(u IngestUpload) Response {
	e.T.Helper()
	digest := u.Digest
	if digest == "" {
		digest = DigestHex(u.Body)
	}
	return e.WebhookFrom(WebhookRequest{
		Method: "PUT", Path: "/ingest/v1/artifacts/" + digest, Headers: u.headers(), Body: u.Body, Chunked: u.Chunked,
	})
}

func (e *Env) IngestStatus(token, digest string) Response {
	e.T.Helper()
	return e.WebhookFrom(WebhookRequest{
		Method: "GET", Path: "/ingest/v1/artifacts/" + digest, Headers: map[string]string{"Authorization": "Bearer " + token},
	})
}

func TarGz(files map[string]string) []byte {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, n := range names {
		h := &tar.Header{Name: n, Mode: 0o644, Size: int64(len(files[n])), Typeflag: tar.TypeReg, ModTime: time.Unix(0, 0)}
		if err := tw.WriteHeader(h); err != nil {
			panic(err)
		}
		if _, err := tw.Write([]byte(files[n])); err != nil {
			panic(err)
		}
	}
	if err := tw.Close(); err != nil {
		panic(err)
	}
	if err := gz.Close(); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func (e *Env) IngestedCount(project uuid.UUID) int {
	e.T.Helper()
	var n int
	require.NoError(e.T, e.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM "IngestedArtifacts" WHERE "ProjectId" = $1 AND NOT "IsDeleted"`, project).Scan(&n))
	return n
}

func (e *Env) BlobStored(digestHex string) bool {
	e.T.Helper()
	d, err := hex.DecodeString(digestHex)
	require.NoError(e.T, err)
	var n int
	require.NoError(e.T, e.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM "Blobs" WHERE "Digest" = $1 AND NOT "IsDeleted"`, d).Scan(&n))
	return n > 0
}

func (e *Env) BlobOnDisk(digestHex string) bool {
	e.T.Helper()
	d, err := blob.ParseDigest(digestHex)
	require.NoError(e.T, err)
	f, err := e.Wired.Artifacts.Store().Open(context.Background(), d)
	if errors.Is(err, blob.ErrNotFound) {
		return false
	}
	require.NoError(e.T, err)
	require.NoError(e.T, f.Close())
	return true
}

func (e *Env) SeedSucceededRelease(project, target uuid.UUID, digestHex, release string, startedAgo time.Duration) {
	e.T.Helper()
	digest, err := hex.DecodeString(digestHex)
	require.NoError(e.T, err)
	run := e.SeedRun(project, "pending")
	_, err = e.Pool.Exec(context.Background(),
		`WITH d AS (
		     INSERT INTO "Deployments" ("OrganizationId", "EnvironmentId", "RunId", "RunNumber", "RunJobId", "JobId", "JobKey", "Ref",
		                                "RuleVersion", "State", "FinishedAt")
		     SELECT t."OrganizationId", t."EnvironmentId", $2, 1, gen_random_uuid(), gen_random_uuid(), 'ship', 'refs/heads/main', 1, 'succeeded', now()
		     FROM "DeployTargets" t WHERE t."Id" = $1
		     RETURNING "Id", "OrganizationId", "RunJobId", "JobId")
		 INSERT INTO "DeploymentReleases" ("OrganizationId", "DeploymentId", "RunJobId", "JobId", "LeaseId", "TargetId", "StepIndex",
		                                   "ReleaseId", "ArtifactDigest", "State", "StartedAt", "FinishedAt")
		 SELECT d."OrganizationId", d."Id", d."RunJobId", d."JobId", gen_random_uuid(), $1, 0, $4, $3, 'succeeded',
		        now() - make_interval(secs => $5::float8), now()
		 FROM d`, target, run, digest, release, startedAgo.Seconds())
	require.NoError(e.T, err)
}

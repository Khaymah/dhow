package integration_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const uniqueViolation = "23505"

func TestSchemaEnforcesTheJobInvariants(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	job, attempt := f.newAttempt(t)
	lease := f.newLease(t)

	tests := []struct {
		name string
		sql  string
		args []any
		code string
	}{
		{
			"a second open attempt for one job", `INSERT INTO "JobAttempts" ("JobId", "OrganizationId", "RunnerId", "Number") VALUES ($1, $2, $3, 2)`,
			[]any{job, f.org, f.runner},
			uniqueViolation,
		},
		{"a reused fencing token", `INSERT INTO "JobAttempts" ("JobId", "OrganizationId", "RunnerId", "Number", "FencingToken")
			SELECT $1, $2, $3, 9, "FencingToken" FROM "JobAttempts" WHERE "Id" = $4`, []any{f.env.SeedJob(f.org, f.creator), f.org, f.runner, attempt}, uniqueViolation},
		{
			"a second lease for one attempt", `INSERT INTO "Leases" ("AttemptId", "JobId", "OrganizationId", "RunnerId", "FencingToken", "GrantDigest", "ExpiresAt")
			SELECT "AttemptId", "JobId", "OrganizationId", "RunnerId", "FencingToken" + 1000, "GrantDigest", "ExpiresAt" FROM "Leases" WHERE "Id" = $1`,
			[]any{lease},
			uniqueViolation,
		},
		{"a second row for one step", `INSERT INTO "AttemptSteps" ("AttemptId", "Index", "Name") VALUES ($1, 0, 'a'), ($1, 0, 'b')`, []any{attempt}, uniqueViolation},
		{"a finished job without a finish time", `UPDATE "Jobs" SET "State" = 'canceled' WHERE "Id" = $1`, []any{job}, checkViolation},
		{"a queued job with a finish time", `UPDATE "Jobs" SET "FinishedAt" = now() WHERE "Id" = $1`, []any{job}, checkViolation},
		{"an unknown job state", `UPDATE "Jobs" SET "State" = 'paused' WHERE "Id" = $1`, []any{job}, checkViolation},
		{"who asked without a cancel request", `UPDATE "Jobs" SET "CancelRequestedBy" = "CreatedByUserId" WHERE "Id" = $1`, []any{job}, checkViolation},
		{"more attempts than retries allow", `UPDATE "Jobs" SET "AttemptCount" = 4 WHERE "Id" = $1`, []any{job}, checkViolation},
		{"a priority above 100", `UPDATE "Jobs" SET "Priority" = 101 WHERE "Id" = $1`, []any{job}, checkViolation},
		{"a short spec digest", `UPDATE "Jobs" SET "SpecDigest" = '\x01' WHERE "Id" = $1`, []any{job}, checkViolation},
		{"a spec that is not an object", `UPDATE "Jobs" SET "Spec" = '[]' WHERE "Id" = $1`, []any{job}, checkViolation},
		{"a running attempt that never started", `UPDATE "JobAttempts" SET "State" = 'running' WHERE "Id" = $1`, []any{attempt}, checkViolation},
		{"a mutation before the start", `UPDATE "JobAttempts" SET "MutationStartedAt" = now() WHERE "Id" = $1`, []any{attempt}, checkViolation},
		{"an ended lease without an end time", `UPDATE "Leases" SET "State" = 'expired' WHERE "Id" = $1`, []any{lease}, checkViolation},
		{"an unknown lease state", `UPDATE "Leases" SET "State" = 'paused', "EndedAt" = now() WHERE "Id" = $1`, []any{lease}, checkViolation},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := f.env.Pool.Exec(ctx, tt.sql, tt.args...)
			require.Error(t, err)
			require.NotNil(t, pgErr(err), err.Error())
			assert.Equal(t, tt.code, pgErr(err).Code, err.Error())
		})
	}
}

func TestFencingTokensOnlyGrow(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	var last int64
	for range 5 {
		_, attempt := f.newAttempt(t)
		var token int64
		require.NoError(t, f.env.Pool.QueryRow(ctx, `SELECT "FencingToken" FROM "JobAttempts" WHERE "Id" = $1`, attempt).Scan(&token))
		assert.Greater(t, token, last)
		last = token
	}
}

const foreignKeyViolation = "23503"

func TestFinishedRowsAreFrozenAndKeysStayInOneOrganization(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	job, attempt := f.newAttempt(t)
	_, err := f.env.Pool.Exec(ctx, setAttemptState, attempt, "running")
	require.NoError(t, err)
	_, err = f.env.Pool.Exec(ctx, setAttemptState, attempt, "succeeded")
	require.NoError(t, err)
	_, err = f.env.Pool.Exec(ctx, setJobState, job, "leased")
	require.NoError(t, err)
	_, err = f.env.Pool.Exec(ctx, setJobState, job, "canceled")
	require.NoError(t, err)

	otherOrg := f.env.SeedOrganization("Other", "other")
	foreignRunner := f.env.SeedRunner(otherOrg, f.creator)
	tests := []struct {
		name string
		sql  string
		args []any
		code string
	}{
		{"renaming a finished job", `UPDATE "Jobs" SET "Name" = 'renamed' WHERE "Id" = $1`, []any{job}, checkViolation},
		{"rewriting the exit code of a finished attempt", `UPDATE "JobAttempts" SET "ExitCode" = 1 WHERE "Id" = $1`, []any{attempt}, checkViolation},
		{"an attempt on a runner of another organization", `INSERT INTO "JobAttempts" ("JobId", "OrganizationId", "RunnerId", "Number")
			VALUES ($1, $2, $3, 1)`, []any{f.env.SeedJob(f.org, f.creator), f.org, foreignRunner}, foreignKeyViolation},
		{"an attempt for a job of another organization", `INSERT INTO "JobAttempts" ("JobId", "OrganizationId", "RunnerId", "Number")
			VALUES ($1, $2, $3, 1)`, []any{f.env.SeedJob(f.org, f.creator), otherOrg, foreignRunner}, foreignKeyViolation},
		{"a lease with another fencing token than its attempt", `INSERT INTO "Leases" ("AttemptId", "JobId", "OrganizationId", "RunnerId",
			"FencingToken", "GrantDigest", "ExpiresAt") SELECT "Id", "JobId", "OrganizationId", "RunnerId", "FencingToken" + 1, sha256('g'),
			now() + interval '1 minute' FROM "JobAttempts" WHERE "Id" = $1`, []any{attempt}, foreignKeyViolation},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := f.env.Pool.Exec(ctx, tt.sql, tt.args...)
			require.Error(t, err)
			require.NotNil(t, pgErr(err), err.Error())
			assert.Equal(t, tt.code, pgErr(err).Code, err.Error())
		})
	}
	_, err = f.env.Pool.Exec(ctx, `UPDATE "Jobs" SET "IsDeleted" = true, "UpdatedAt" = now() WHERE "Id" = $1`, job)
	assert.NoError(t, err, "a soft delete of a finished job is allowed")
}

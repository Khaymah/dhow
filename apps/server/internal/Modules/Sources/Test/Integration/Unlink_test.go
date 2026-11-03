package integration_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	sources "github.com/Khaymah/dhow/apps/server/internal/Modules/Sources"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Sources/Models"
)

func unlink(t *testing.T, s setup) {
	t.Helper()
	res := s.owner.Do(apptest.Request{Method: http.MethodDelete, Path: apptest.OrgPath(s.org, "/projects/"+s.project.String()+"/repository")})
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
}

func TestAnUnlinkedRepositoryReportsNoCommitStatus(t *testing.T) {
	s := linkedRepository(t)
	fake := &fakeProvider{}
	svc := service(t, s, map[string]sources.Provider{models.ProviderGeneric: fake})
	record(t, s, svc, uuid.New(), sources.StatusPending)
	_, err := s.env.Pool.Exec(context.Background(), `UPDATE "Repositories" SET "IsDeleted" = true, "WebhookSecretSealed" = NULL WHERE "Id" = $1`, s.repo)
	require.NoError(t, err)
	pass, err := svc.SendStatuses(context.Background())
	require.NoError(t, err)
	assert.Equal(t, sources.StatusPass{}, pass)
	assert.Empty(t, fake.sent())
}

func TestUnlinkAbandonsTheUnsentCommitStatusesAndErasesTheSecrets(t *testing.T) {
	s := linkedRepository(t)
	fake := &fakeProvider{}
	svc := service(t, s, map[string]sources.Provider{models.ProviderGeneric: fake})
	record(t, s, svc, uuid.New(), sources.StatusPending)
	unlink(t, s)
	assert.Equal(t, 1, s.env.CountRows(`SELECT count(*) FROM "CommitStatusReports" WHERE "AbandonedAt" IS NOT NULL`))
	pass, err := svc.SendStatuses(context.Background())
	require.NoError(t, err)
	assert.Equal(t, sources.StatusPass{}, pass)
	assert.Empty(t, fake.sent())
	assert.Equal(t, 1, s.env.CountRows(`SELECT count(*) FROM "Repositories" WHERE "IsDeleted" AND "DeployKeySealed" IS NULL
		AND "PreviousDeployKeySealed" IS NULL AND "WebhookSecretSealed" IS NULL AND "PendingHostKey" IS NULL`))
}

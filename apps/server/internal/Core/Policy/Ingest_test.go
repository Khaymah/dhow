package policy

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIngestRules(t *testing.T) {
	assert.True(t, Ingest(http.MethodPut, "/ingest/v1/artifacts/{sha256}").Valid())
	assert.True(t, Ingest(http.MethodGet, "/ingest/v1/artifacts/{sha256}").Valid())
	invalid := []Rule{
		Ingest(http.MethodPost, "/ingest/v1/artifacts/{sha256}"),
		Ingest(http.MethodDelete, "/ingest/v1/artifacts/{sha256}"),
		Ingest(http.MethodPut, "/api/v1/artifacts"),
		Ingest(http.MethodPut, "/ingest/v1/"),
		Ingest(http.MethodPut, "/ingest/v1/artifacts/{sha256}").AllowTokens(),
		{Method: http.MethodPut, Path: "/ingest/v1/artifacts/x", Scope: ScopeIngest, Action: IngestManage},
		{Method: http.MethodPut, Path: "/ingest/v1/artifacts/x", Scope: ScopeIngest, OrgParam: "id"},
		{Method: http.MethodPut, Path: "/ingest/v1/artifacts/x", Scope: ScopeIngest, ProjectParam: "projectId"},
		{Method: http.MethodPut, Path: "/ingest/v1/artifacts/x", Scope: ScopeIngest, Runner: RunnerSigned},
	}
	for _, r := range invalid {
		assert.False(t, r.Valid(), "%+v", r)
	}
	table := NewTable(Ingest(http.MethodPut, "/ingest/v1/artifacts/{sha256}"))
	_, ok := table.Lookup(http.MethodPut, "/ingest/v1/artifacts/{sha256}")
	assert.True(t, ok)
	_, ok = table.Lookup(http.MethodPut, "/ingest/v1/artifacts")
	assert.False(t, ok)
}

func TestIngestManageIsAProjectActionForMaintainersAndAbove(t *testing.T) {
	assert.True(t, IsProjectAction(IngestManage))
	assert.True(t, IsSensitive(IngestManage))
	assert.False(t, IsTokenAction(IngestManage), "an API token never creates an ingest token")
	assert.Contains(t, Actions(), IngestManage)
	for role, want := range map[Role]bool{
		RoleOwner: true, RoleAdmin: true, RoleMaintainer: true, RoleDeveloper: false, RoleViewer: false,
	} {
		assert.Equal(t, want, Allows(role, IngestManage), string(role))
	}
}

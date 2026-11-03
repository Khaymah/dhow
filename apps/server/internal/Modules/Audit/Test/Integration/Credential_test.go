package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	audit "github.com/Khaymah/dhow/apps/server/internal/Modules/Audit"
)

func storedData(t *testing.T, f fixture, action string) map[string]string {
	t.Helper()
	var raw []byte
	require.NoError(t, f.pool.QueryRow(context.Background(), `SELECT "Data" FROM "AuditEvents" WHERE "Action" = $1`, action).Scan(&raw))
	var data map[string]string
	require.NoError(t, json.Unmarshal(raw, &data))
	return data
}

func TestTheTokenThatActedIsNeverCutByTheDataCap(t *testing.T) {
	f := newFixture(t)
	token := uuid.New()
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{Subject: policy.Subject{UserID: uuid.New(), TokenID: token}})
	data := map[string]string{"Via": "browser", "ViaId": "forged"}
	keys := audit.AllowedDataKeys()
	require.Greater(t, len(keys), 40)
	for i := range 40 {
		data[keys[i]] = fmt.Sprintf("value %d", i)
	}
	require.NoError(t, f.service.Handle(ctx, eventbus.Event{Name: "test.many_keys", Payload: eventbus.Audited{
		OrganizationIDs: []uuid.UUID{f.org}, TargetType: "test", TargetID: "x", Data: data,
	}}))

	stored := storedData(t, f, "test.many_keys")
	assert.Equal(t, "api_token", stored["Via"])
	assert.Equal(t, token.String(), stored["ViaId"])
	assert.Len(t, stored, 32+2, "the cap applies to the event's own data, and the credential comes on top")
}

func TestEventsCannotClaimACredential(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, f.service.Handle(context.Background(), eventbus.Event{Name: "test.claim", Payload: eventbus.Audited{
		OrganizationIDs: []uuid.UUID{f.org}, TargetType: "test", TargetID: "x", Data: map[string]string{"Via": "api_token", "ViaId": "forged", "Name": "1"},
	}}))
	assert.Equal(t, map[string]string{"Name": "1"}, storedData(t, f, "test.claim"), "only the request's principal sets Via")
}

package app_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
)

func dataKeysInUse(t *testing.T, env *apptest.Env) map[uuid.UUID]bool {
	t.Helper()
	out := map[uuid.UUID]bool{}
	for _, values := range env.AllValues(t) {
		for _, v := range values {
			id, ok := keys.DataKeyIDOf(v.Sealed)
			require.True(t, ok)
			out[id] = true
		}
	}
	return out
}

func TestKeysRekeyMovesEveryStoredValueToNewDataKeys(t *testing.T) {
	w := newSealedWorld(t)
	before := dataKeysInUse(t, w.env)
	require.Len(t, before, 4, "one data key for the instance, the user and each of the two projects")
	cli := &keysCLI{t: t, pool: w.env.Pool, file: keyringFile(t, apptest.TestKeyring(t))}

	require.NoError(t, cli.run("rekey", "--grace", "0s"))
	out := cli.out.String()
	assert.Contains(t, out, "4 owners have a new data key, 7 values were sealed again and 4 old data keys were destroyed")
	for _, m := range keyMaterial(apptest.TestKeyring(t)) {
		assert.NotContains(t, out, m)
	}
	after := dataKeysInUse(t, w.env)
	require.Len(t, after, 4)
	for id := range after {
		assert.False(t, before[id], "no value keeps a data key from before the rekey")
	}
	w.everyValueWorks(t, w.env)
	assert.Equal(t, []string{"system.keys.rekeyed"}, auditActions(t, w.env.Pool))
	var severity, destroyed string
	require.NoError(t, w.env.Pool.QueryRow(t.Context(), `SELECT "Data"->>'Severity', "Data"->>'Destroyed' FROM "AuditEvents"
		WHERE "Action" = 'system.keys.rekeyed'`).Scan(&severity, &destroyed))
	assert.Equal(t, "high", severity)
	assert.Equal(t, "4", destroyed)
	assert.False(t, strings.Contains(out, "Resumed"))
}

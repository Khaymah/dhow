package e2e_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
)

type callerView struct {
	Id         string
	CallerRole *string
}

func callerRole(t *testing.T, res apptest.Response) string {
	t.Helper()
	require.Equal(t, http.StatusOK, res.Status, string(res.Raw))
	var v callerView
	res.Decode(t, &v)
	require.NotNil(t, v.CallerRole, string(res.Raw))
	return *v.CallerRole
}

func TestAProjectTellsTheCallerItsRoleAfterTheBindings(t *testing.T) {
	w := newWorld(t)
	bound, other := w.env.SeedProject(w.org, w.ownerID), w.env.SeedProject(w.org, w.ownerID)
	dev, devID := w.member(t, "developer")
	w.env.SeedUserProjectBinding(w.org, bound, devID, "maintainer")
	maintainer, maintainerID := w.member(t, "maintainer")
	w.env.SeedUserProjectBinding(w.org, bound, maintainerID, "viewer")

	cases := []struct {
		name   string
		client *apptest.Client
		bound  string
		other  string
	}{
		{"the owner", w.owner, "owner", "owner"},
		{"a developer raised to maintainer", dev, "maintainer", "developer"},
		{"a maintainer lowered to viewer", maintainer, "viewer", "maintainer"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.bound, callerRole(t, c.client.Get(w.path("/"+bound.String()))))
			assert.Equal(t, c.other, callerRole(t, c.client.Get(w.path("/"+other.String()))))
		})
	}

	renamed := w.owner.Do(apptest.Request{Method: http.MethodPatch, Path: w.path("/" + bound.String()), Body: map[string]any{"Name": "Renamed"}})
	assert.Equal(t, "owner", callerRole(t, renamed))
	archived := w.owner.Do(apptest.Request{Method: http.MethodPost, Path: w.path("/" + bound.String() + "/archive")})
	assert.Equal(t, "owner", callerRole(t, archived))

	list := w.owner.Get(w.path(""))
	require.Equal(t, http.StatusOK, list.Status, string(list.Raw))
	assert.NotContains(t, string(list.Raw), "CallerRole")
}

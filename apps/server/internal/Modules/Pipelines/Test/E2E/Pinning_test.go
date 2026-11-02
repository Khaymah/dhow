package e2e_test

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	triggers "github.com/Khaymah/dhow/apps/server/internal/Core/Triggers"
	"github.com/Khaymah/dhow/internal/netguard"
)

func TestTheServerPinsATagFromARegistryBehindTheGuard(t *testing.T) {
	digest := "sha256:" + repeat("d", 64)
	var asked atomic.Int64
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/team/tool/manifests/2" {
			http.NotFound(w, r)
			return
		}
		asked.Add(1)
		w.Header().Set("Docker-Content-Digest", digest)
	}))
	t.Cleanup(srv.Close)
	_, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	require.NoError(t, err)
	transport, ok := srv.Client().Transport.(*http.Transport)
	require.True(t, ok)
	defs := apptest.NewFakeDefinitions()
	defs.Put(shaA, "version: 1\non:\n  push: {}\njobs:\n  build:\n    image: registry.example.com/team/tool:2\n    steps:\n      - run: make\n")
	env := apptest.NewWithOptions(t, apptest.Options{
		Definitions: defs,
		RegistryTLS: &tls.Config{RootCAs: transport.TLSClientConfig.RootCAs, MinVersion: tls.VersionTLS12},
		GitLookup: func(context.Context, string) ([]netip.Addr, error) {
			return []netip.Addr{netip.MustParseAddr("10.255.0.30")}, nil
		},
		GitDial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, net.JoinHostPort("127.0.0.1", port))
		},
		Profile: func(c *config.Config) {
			allowed, err := netguard.ParseCIDRs("10.255.0.0/16")
			require.NoError(t, err)
			c.Sources.AllowPrivate = allowed
		},
	})
	owner, d := env.SetupAndLogin()
	org, ownerID := env.OrganizationID(d.OrgSlug), uuid.MustParse(owner.UserID)
	project := env.SeedProject(org, ownerID)
	repo := env.SeedRepository(org, project, ownerID)
	got := env.Deliver(triggers.Trigger{
		OrganizationID: org, ProjectID: project, RepositoryID: repo, DeliveryID: uuid.New(), Provider: "generic",
		Event: triggers.EventPush, Ref: "refs/heads/main", Sha: shaA,
	})
	env.StartRuns()
	pinned := "registry.example.com/team/tool:2@sha256:" + repeat("d", 64)
	w := world{env: env, owner: owner, org: org, project: project}
	v := w.view(t, got.RunID)
	require.Equal(t, "running", v.Run.State, "%+v", v.Issues)
	require.Len(t, v.ImagePins, 1)
	assert.Equal(t, pinned, v.ImagePins[0].Pinned)
	assert.Equal(t, pinned, w.jobImage(t, *env.RunJobs(got.RunID)["build"].Job))
	assert.Equal(t, int64(1), asked.Load(), "one registry call per tag per run")
}

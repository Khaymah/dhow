package auth_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
)

type fakeProjects struct {
	mu       sync.Mutex
	bindings map[uuid.UUID][]policy.Role
	calls    int
	err      error
}

func (f *fakeProjects) ProjectBindings(_ context.Context, _, project, _ uuid.UUID) ([]policy.Role, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	roles, ok := f.bindings[project]
	if !ok {
		return nil, auth.ErrNoProject
	}
	return roles, nil
}

type accessRig struct {
	guard    *auth.Guard
	resolver *liveResolver
	projects *fakeProjects
	ctx      context.Context
	hash     []byte
	org      uuid.UUID
}

func newAccessRig(t *testing.T) accessRig {
	t.Helper()
	cookie, hash, err := auth.NewSessionToken(config.SessionTokenBytes)
	require.NoError(t, err)
	resolver := &liveResolver{sessions: map[string]auth.Resolved{}}
	projects := &fakeProjects{bindings: map[uuid.UUID][]policy.Role{}}
	u, err := url.Parse(origin)
	require.NoError(t, err)
	guard := auth.NewGuard(auth.GuardOptions{
		Resolver: resolver, Projects: projects, CSRF: auth.NewCSRF([]byte("0123456789abcdef0123456789abcdef")), PublicURL: u,
		Log: slog.New(slog.NewJSONHandler(io.Discard, nil)),
	})
	return accessRig{
		guard: guard, resolver: resolver, projects: projects, hash: hash, org: uuid.New(),
		ctx: auth.WithRequestMeta(context.Background(), auth.RequestMeta{SessionToken: cookie}),
	}
}

func (r accessRig) sign(role policy.Role, org uuid.UUID) {
	r.resolver.set(r.hash, &auth.Resolved{
		UserID: uuid.New(), SessionID: uuid.New(), Memberships: []policy.Membership{{OrganizationID: org, Role: role}},
	}, nil)
}

func codeOfAccess(err error) string {
	var ae *apperrors.AppError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return ""
}

func TestAccessFollowsTheCurrentMembershipAndSession(t *testing.T) {
	r := newAccessRig(t)
	r.sign(policy.RoleDeveloper, r.org)
	_, err := r.guard.AccessIn(r.ctx, r.org)
	require.NoError(t, err)
	assert.Equal(t, 1, r.resolver.peeks, "it peeks and never resolves, so the session is not extended")

	r.sign(policy.RoleOwner, uuid.New())
	_, err = r.guard.AccessIn(r.ctx, r.org)
	assert.Equal(t, apperrors.CodeNotFound, codeOfAccess(err), "a removed member is no member")

	r.resolver.set(r.hash, nil, nil)
	_, err = r.guard.AccessIn(r.ctx, r.org)
	assert.Equal(t, apperrors.CodeUnauthenticated, codeOfAccess(err))

	_, err = r.guard.AccessIn(context.Background(), r.org)
	assert.Equal(t, apperrors.CodeUnauthenticated, codeOfAccess(err), "a context without the session cookie never passes")

	r.resolver.set(r.hash, &auth.Resolved{UserID: uuid.New(), Memberships: []policy.Membership{{OrganizationID: r.org, Role: policy.RoleOwner}}}, errors.New("database is down"))
	_, err = r.guard.AccessIn(r.ctx, r.org)
	assert.Equal(t, apperrors.CodeInternal, codeOfAccess(err))
}

func TestAccessCanTellsWhatTheCurrentRoleMayDoInAnOrganizationAndAProject(t *testing.T) {
	r := newAccessRig(t)
	project := uuid.New()
	r.projects.bindings[project] = nil

	for _, tt := range []struct {
		role   policy.Role
		action policy.Action
		want   bool
	}{
		{policy.RoleDeveloper, policy.DeploymentApprove, true},
		{policy.RoleViewer, policy.DeploymentApprove, false},
		{policy.RoleViewer, policy.RunRead, true},
		{policy.RoleViewer, policy.ProjectRead, true},
		{policy.RoleMaintainer, policy.DeploymentApprove, true},
	} {
		r.sign(tt.role, r.org)
		access, err := r.guard.AccessIn(r.ctx, r.org)
		require.NoError(t, err)
		got, err := access.Can(r.ctx, tt.action, project)
		require.NoError(t, err)
		assert.Equal(t, tt.want, got, "%s may %s in a project: %t", tt.role, tt.action, tt.want)
	}

	r.sign(policy.RoleViewer, r.org)
	access, err := r.guard.AccessIn(r.ctx, r.org)
	require.NoError(t, err)
	orgLevel, err := access.Can(r.ctx, policy.OrgRead, uuid.Nil)
	require.NoError(t, err)
	assert.True(t, orgLevel)
	denied, err := access.Can(r.ctx, policy.MemberManage, uuid.Nil)
	require.NoError(t, err)
	assert.False(t, denied)
}

func TestAProjectBindingRaisesTheRoleAndIsLookedUpOncePerProject(t *testing.T) {
	r := newAccessRig(t)
	project := uuid.New()
	r.projects.bindings[project] = []policy.Role{policy.RoleMaintainer}
	r.sign(policy.RoleViewer, r.org)
	access, err := r.guard.AccessIn(r.ctx, r.org)
	require.NoError(t, err)

	for range 3 {
		ok, err := access.Can(r.ctx, policy.DeploymentApprove, project)
		require.NoError(t, err)
		assert.True(t, ok, "a viewer bound as maintainer on the project may decide there")
	}
	assert.Equal(t, 1, r.projects.calls, "the bindings of a project are read once for the life of one access check")
}

func TestAProjectThatIsGoneOrFailsToLoadIsNeverAllowed(t *testing.T) {
	r := newAccessRig(t)
	r.sign(policy.RoleOwner, r.org)
	access, err := r.guard.AccessIn(r.ctx, r.org)
	require.NoError(t, err)

	ok, err := access.Can(r.ctx, policy.RunRead, uuid.New())
	require.NoError(t, err)
	assert.False(t, ok, "a project that is not in the organization is not readable")

	r.projects.err = errors.New("database is down")
	ok, err = access.Can(r.ctx, policy.RunRead, uuid.New())
	require.Error(t, err)
	assert.False(t, ok)
}

func TestAnApproverPastTheSecondFactorDeadlineStopsGettingWhatNeedsTheFactor(t *testing.T) {
	r := newAccessRig(t)
	project := uuid.New()
	r.projects.bindings[project] = nil
	resolved := func(due time.Time) *auth.Resolved {
		return &auth.Resolved{
			UserID: uuid.New(), SessionID: uuid.New(),
			Memberships: []policy.Membership{{OrganizationID: r.org, Role: policy.RoleDeveloper, SecondFactorDue: due}},
		}
	}
	for _, tt := range []struct {
		name string
		due  time.Time
		want bool
	}{
		{"no deadline", time.Time{}, true},
		{"a deadline ahead", time.Now().Add(time.Hour), true},
		{"a deadline passed", time.Now().Add(-time.Hour), false},
	} {
		r.resolver.set(r.hash, resolved(tt.due), nil)
		access, err := r.guard.AccessIn(r.ctx, r.org)
		require.NoError(t, err, tt.name)
		approve, err := access.Can(r.ctx, policy.DeploymentApprove, project)
		require.NoError(t, err)
		assert.Equal(t, tt.want, approve, tt.name)
		read, err := access.Can(r.ctx, policy.RunRead, project)
		require.NoError(t, err)
		assert.True(t, read, "%s: reading is not a sensitive action", tt.name)
	}
}

package apptest

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"github.com/Khaymah/dhow/internal/sshhostkey"
)

const (
	RouteTableTarget     = "route-table-target"
	RouteTableTargetHost = "10.0.0.5"
)

func (e *Env) SeedDeployTarget(project uuid.UUID, environment, name string) (uuid.UUID, uuid.UUID) {
	e.T.Helper()
	var target, host uuid.UUID
	err := e.Pool.QueryRow(context.Background(),
		`SELECT "Id" FROM "DeployTargets" WHERE "ProjectId" = $1 AND "Name" = $2 AND NOT "IsDeleted"`, project, name).Scan(&target)
	if err == nil {
		require.NoError(e.T, e.Pool.QueryRow(context.Background(),
			`SELECT "Id" FROM "DeployTargetHosts" WHERE "TargetId" = $1 AND NOT "IsDeleted" ORDER BY "Position" LIMIT 1`, target).Scan(&host))
		return target, host
	}
	env := e.SeedEnvironment(project, environment)
	require.NoError(e.T, e.Pool.QueryRow(context.Background(),
		`INSERT INTO "DeployTargets" ("OrganizationId", "ProjectId", "EnvironmentId", "Name", "Kind", "Username", "DeployPath", "CreatedByUserId")
		 SELECT p."OrganizationId", p."Id", $2, $3, 'ssh', 'deploy', '/srv/rt-' || p."Id"::text, p."CreatedByUserId"
		 FROM "Projects" p WHERE p."Id" = $1
		 RETURNING "Id"`, project, env, name).Scan(&target))
	require.NoError(e.T, e.Pool.QueryRow(context.Background(),
		`INSERT INTO "DeployTargetHosts" ("OrganizationId", "TargetId", "Position", "Host", "Port", "DeployPath")
		 SELECT "OrganizationId", "Id", 1, $2, 22, "DeployPath" FROM "DeployTargets" WHERE "Id" = $1
		 RETURNING "Id"`, target, RouteTableTargetHost).Scan(&host))
	return target, host
}

type SeededHost struct {
	ID      uuid.UUID
	Host    string
	HostKey string
}

type SeededTarget struct {
	ID    uuid.UUID
	Hosts []SeededHost
}

const SeedTargetSettings = `{"Rolling":{"BatchSize":2,"BatchPauseSeconds":30},"Ssh":{"ReloadCommand":"sudo systemctl reload web","PreSwitchCommand":"","PostSwitchCommand":"","Shared":[".env","storage"]}}`

func NewHostKeyLine(t require.TestingT) (string, string) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	key, err := ssh.NewPublicKey(pub)
	require.NoError(t, err)
	return sshhostkey.AuthorizedLine(key), ssh.FingerprintSHA256(key)
}

func (e *Env) SeedPinnedTarget(project uuid.UUID, environment, name, keySecret string, hosts []string) SeededTarget {
	e.T.Helper()
	ctx := context.Background()
	env := e.SeedEnvironment(project, environment)
	var out SeededTarget
	require.NoError(e.T, e.Pool.QueryRow(ctx,
		`INSERT INTO "DeployTargets" ("OrganizationId", "ProjectId", "EnvironmentId", "Name", "Kind", "Username", "KeySecretName", "DeployPath",
		        "Settings", "CreatedByUserId")
		 SELECT p."OrganizationId", p."Id", $2, $3, 'ssh', 'deploy', $4, '/srv/' || $3::text, $5::jsonb, p."CreatedByUserId"
		 FROM "Projects" p WHERE p."Id" = $1
		 RETURNING "Id"`, project, env, name, keySecret, SeedTargetSettings).Scan(&out.ID))
	for i, host := range hosts {
		line, fingerprint := NewHostKeyLine(e.T)
		h := SeededHost{Host: host, HostKey: line}
		require.NoError(e.T, e.Pool.QueryRow(ctx,
			`INSERT INTO "DeployTargetHosts" ("OrganizationId", "TargetId", "Position", "Host", "Port", "DeployPath", "HostKey", "HostKeyFingerprint",
			        "HostKeyConfirmedAt", "HostKeyConfirmedBy")
			 SELECT t."OrganizationId", t."Id", $2, $3, 22, t."DeployPath", $4, $5, now(), t."CreatedByUserId" FROM "DeployTargets" t WHERE t."Id" = $1
			 RETURNING "Id"`, out.ID, i+1, host, line, fingerprint).Scan(&h.ID))
		out.Hosts = append(out.Hosts, h)
	}
	return out
}

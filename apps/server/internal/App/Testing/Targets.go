package apptest

import (
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
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

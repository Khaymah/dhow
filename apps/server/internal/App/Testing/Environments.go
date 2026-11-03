package apptest

import (
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

const RouteTableEnvironment = "production"

func (e *Env) SeedEnvironment(project uuid.UUID, name string) uuid.UUID {
	e.T.Helper()
	var id uuid.UUID
	require.NoError(e.T, e.Pool.QueryRow(context.Background(),
		`INSERT INTO "Environments" ("OrganizationId", "ProjectId", "Name", "AllowedRefs", "CreatedByUserId")
		 SELECT p."OrganizationId", p."Id", $2, '[{"Kind":"branch","Pattern":"main"}]', p."CreatedByUserId"
		 FROM "Projects" p WHERE p."Id" = $1
		 ON CONFLICT ("ProjectId", "Name") WHERE NOT "IsDeleted" DO UPDATE SET "UpdatedAt" = EXCLUDED."UpdatedAt"
		 RETURNING "Id"`,
		project, name).Scan(&id))
	return id
}

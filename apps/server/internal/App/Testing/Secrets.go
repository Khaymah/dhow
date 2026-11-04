package apptest

import (
	"context"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Secrets/Models"
)

const (
	RouteTableSecret   = "ROUTE_TABLE_SECRET" //nolint:gosec // A secret name for tests, not a credential.
	RouteTableVariable = "ROUTE_TABLE_VARIABLE"
	RouteTableRegistry = "registry.example.com"
)

func SecretRef(org uuid.UUID, project *uuid.UUID, environment string) models.Ref {
	return models.Ref{OrganizationID: org, ProjectID: project, EnvironmentName: environment}
}

func (e *Env) actorContext(org uuid.UUID) context.Context {
	e.T.Helper()
	var owner uuid.UUID
	require.NoError(e.T, e.Pool.QueryRow(context.Background(),
		`SELECT "UserId" FROM "Memberships" WHERE "OrganizationId" = $1 AND "Role" = 'owner' AND NOT "IsDeleted" LIMIT 1`, org).Scan(&owner))
	return auth.WithPrincipal(context.Background(), auth.Principal{Subject: policy.Subject{UserID: owner}})
}

func (e *Env) SeedSecret(ref models.Ref, name, value string) models.Secret {
	e.T.Helper()
	s, err := e.Wired.Options.Secrets.PutSecret(e.actorContext(ref.OrganizationID), ref, name, models.PutSecretInput{Value: value})
	require.NoError(e.T, err)
	return s
}

func (e *Env) SeedVariable(ref models.Ref, name, value string) models.Variable {
	e.T.Helper()
	v, err := e.Wired.Options.Secrets.PutVariable(e.actorContext(ref.OrganizationID), ref, name, value)
	require.NoError(e.T, err)
	return v
}

func (e *Env) SeedRegistryCredential(ref models.Ref, host, username, password string) models.RegistryCredential {
	e.T.Helper()
	r, err := e.Wired.Options.Secrets.CreateRegistryCredential(e.actorContext(ref.OrganizationID), ref,
		models.RegistryInput{Host: host, Username: username, Password: password})
	require.NoError(e.T, err)
	return r
}

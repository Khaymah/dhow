-- name: NotifyOrgEvent :exec
SELECT pg_notify('dhow_org_events', @payload::text);

-- name: ListenOrgEvents :exec
LISTEN dhow_org_events;

-- name: EventTargetScope :one
SELECT "ProjectId", "EnvironmentId"
FROM "DeployTargets"
WHERE "Id" = @id AND "OrganizationId" = @organization_id;

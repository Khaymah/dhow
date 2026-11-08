-- name: NotificationScopeProject :one
SELECT "State" FROM "Projects" WHERE "Id" = @project_id AND "OrganizationId" = @organization_id AND NOT "IsDeleted";

-- name: LockNotificationScope :exec
SELECT pg_advisory_xact_lock(hashtextextended('dhow.notification-channels:' || @scope::text, 0));

-- name: InsertNotificationChannel :one
INSERT INTO "NotificationChannels" (
    "Id", "OrganizationId", "ProjectId", "Kind", "Name", "Locale", "Host", "ConfigSealed", "CreatedByUserId"
)
VALUES (@id, @organization_id, sqlc.narg('project_id'), @kind, @name, @locale, @host, sqlc.narg('config_sealed'), @created_by_user_id)
RETURNING *;

-- name: GetNotificationChannel :one
SELECT * FROM "NotificationChannels"
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND "ProjectId" IS NOT DISTINCT FROM sqlc.narg('project_id')::uuid AND NOT "IsDeleted";

-- name: GetNotificationChannelForUpdate :one
SELECT * FROM "NotificationChannels"
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND "ProjectId" IS NOT DISTINCT FROM sqlc.narg('project_id')::uuid AND NOT "IsDeleted"
FOR UPDATE;

-- name: GetNotificationChannelByID :one
SELECT * FROM "NotificationChannels" WHERE "Id" = @id AND NOT "IsDeleted";

-- name: ListNotificationChannels :many
SELECT * FROM "NotificationChannels"
WHERE "OrganizationId" = @organization_id AND "ProjectId" IS NOT DISTINCT FROM sqlc.narg('project_id')::uuid AND NOT "IsDeleted"
ORDER BY "CreatedAt", "Id"
LIMIT @page_limit OFFSET @page_offset;

-- name: CountNotificationChannels :one
SELECT count(*) FROM "NotificationChannels"
WHERE "OrganizationId" = @organization_id AND "ProjectId" IS NOT DISTINCT FROM sqlc.narg('project_id')::uuid AND NOT "IsDeleted";

-- name: NotificationChannelNameTaken :one
SELECT EXISTS (
    SELECT 1 FROM "NotificationChannels"
    WHERE "OrganizationId" = @organization_id AND "ProjectId" IS NOT DISTINCT FROM sqlc.narg('project_id')::uuid
      AND lower("Name") = lower(@name::text) AND NOT "IsDeleted" AND "Id" <> @except_id
);

-- name: UpdateNotificationChannel :one
UPDATE "NotificationChannels"
SET "Name" = @name, "Locale" = @locale, "Version" = "Version" + 1, "UpdatedAt" = now()
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND "Version" = @version AND NOT "IsDeleted"
RETURNING *;

-- name: ReplaceNotificationChannelConfig :one
UPDATE "NotificationChannels"
SET "ConfigSealed" = @config_sealed, "Host" = @host, "ConsecutiveFailures" = 0, "Version" = "Version" + 1, "UpdatedAt" = now()
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND NOT "IsDeleted"
RETURNING *;

-- name: EnableNotificationChannel :one
UPDATE "NotificationChannels"
SET "Enabled" = true, "DisabledReason" = NULL, "ConsecutiveFailures" = 0, "CreatedByUserId" = @actor_id,
    "Version" = "Version" + 1, "UpdatedAt" = now()
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND NOT "IsDeleted"
RETURNING *;

-- name: RemoveNotificationChannel :one
UPDATE "NotificationChannels"
SET "IsDeleted" = true, "UpdatedAt" = now()
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND NOT "IsDeleted"
RETURNING *;

-- name: RemoveNotificationRulesOfChannel :exec
UPDATE "NotificationRules" SET "IsDeleted" = true, "UpdatedAt" = now() WHERE "ChannelId" = @channel_id AND NOT "IsDeleted";

-- name: RecordNotificationSuccess :exec
UPDATE "NotificationChannels" SET "ConsecutiveFailures" = 0, "UpdatedAt" = now()
WHERE "Id" = @id AND "ConsecutiveFailures" <> 0;

-- name: RecordNotificationFailure :one
UPDATE "NotificationChannels" SET "ConsecutiveFailures" = "ConsecutiveFailures" + 1, "UpdatedAt" = now()
WHERE "Id" = @id AND NOT "IsDeleted"
RETURNING "ConsecutiveFailures", "Enabled";

-- name: DisableNotificationChannel :one
UPDATE "NotificationChannels"
SET "Enabled" = false, "DisabledReason" = @reason::text, "Version" = "Version" + 1, "UpdatedAt" = now()
WHERE "Id" = @id AND "Enabled" AND NOT "IsDeleted"
RETURNING *;

-- name: DisableNotificationChannelsOfCreator :many
UPDATE "NotificationChannels"
SET "Enabled" = false, "DisabledReason" = 'creator-removed', "Version" = "Version" + 1, "UpdatedAt" = now()
WHERE "OrganizationId" = @organization_id AND "CreatedByUserId" = @user_id AND "Enabled" AND NOT "IsDeleted"
RETURNING *;

-- name: ListNotificationCreatorAccess :many
SELECT u."Id" AS "UserId", m."Role",
       ARRAY(SELECT r."Role" FROM "OrganizationTeamRoles" r
             WHERE r."UserId" = u."Id" AND r."OrganizationId" = m."OrganizationId")::text[] AS "TeamRoles",
       ARRAY(SELECT rb."Role"
             FROM "RoleBindings" rb
             WHERE rb."OrganizationId" = m."OrganizationId" AND rb."Scope" = 'project' AND rb."ProjectId" = sqlc.narg('project_id')::uuid
               AND NOT rb."IsDeleted"
               AND (rb."UserId" = u."Id"
                    OR EXISTS (SELECT 1
                               FROM "TeamMembers" tm
                               JOIN "Teams" t ON t."Id" = tm."TeamId" AND t."OrganizationId" = tm."OrganizationId" AND NOT t."IsDeleted"
                               WHERE tm."TeamId" = rb."TeamId" AND tm."OrganizationId" = rb."OrganizationId"
                                 AND tm."UserId" = u."Id" AND NOT tm."IsDeleted")))::text[] AS "Bindings"
FROM "Users" u
JOIN "Memberships" m ON m."UserId" = u."Id" AND m."OrganizationId" = @organization_id AND NOT m."IsDeleted"
JOIN "Organizations" o ON o."Id" = m."OrganizationId" AND NOT o."IsDeleted"
WHERE u."Id" = ANY (@user_ids::uuid[]) AND NOT u."IsDeleted" AND NOT u."Disabled";

-- name: GetNotificationRule :one
SELECT * FROM "NotificationRules" WHERE "ChannelId" = @channel_id AND NOT "IsDeleted";

-- name: ListNotificationRules :many
SELECT r.* FROM "NotificationRules" r
JOIN "NotificationChannels" c ON c."Id" = r."ChannelId" AND c."OrganizationId" = r."OrganizationId" AND NOT c."IsDeleted"
WHERE r."OrganizationId" = @organization_id AND c."ProjectId" IS NOT DISTINCT FROM sqlc.narg('project_id')::uuid AND NOT r."IsDeleted"
ORDER BY r."CreatedAt", r."Id";

-- name: InsertNotificationRule :one
INSERT INTO "NotificationRules" ("OrganizationId", "ProjectId", "ChannelId", "Events", "Environments", "Statuses", "Roles")
VALUES (@organization_id, sqlc.narg('project_id'), @channel_id, @events, @environments, @statuses, @roles)
RETURNING *;

-- name: UpdateNotificationRule :one
UPDATE "NotificationRules"
SET "Events" = @events, "Environments" = @environments, "Statuses" = @statuses, "Roles" = @roles,
    "Version" = "Version" + 1, "UpdatedAt" = now()
WHERE "ChannelId" = @channel_id AND NOT "IsDeleted"
RETURNING *;

-- name: ListRoutingRules :many
SELECT r."ChannelId", r."Events", r."Environments", r."Statuses", r."Roles",
       c."Kind", c."Locale", c."Enabled", c."ProjectId" AS "ChannelProjectId"
FROM "NotificationRules" r
JOIN "NotificationChannels" c ON c."Id" = r."ChannelId" AND c."OrganizationId" = r."OrganizationId"
WHERE r."OrganizationId" = @organization_id AND NOT r."IsDeleted" AND NOT c."IsDeleted" AND c."Enabled"
  AND (c."ProjectId" IS NULL OR c."ProjectId" = sqlc.narg('project_id')::uuid)
ORDER BY r."CreatedAt", r."Id";

-- name: ListNotificationRecipients :many
SELECT u."Id" AS "UserId", m."Role",
       ARRAY(SELECT r."Role" FROM "OrganizationTeamRoles" r
             WHERE r."UserId" = u."Id" AND r."OrganizationId" = m."OrganizationId")::text[] AS "TeamRoles",
       ARRAY(SELECT rb."Role"
             FROM "RoleBindings" rb
             WHERE rb."OrganizationId" = m."OrganizationId" AND rb."Scope" = 'project' AND rb."ProjectId" = sqlc.narg('project_id')::uuid
               AND NOT rb."IsDeleted"
               AND (rb."UserId" = u."Id"
                    OR EXISTS (SELECT 1
                               FROM "TeamMembers" tm
                               JOIN "Teams" t ON t."Id" = tm."TeamId" AND t."OrganizationId" = tm."OrganizationId" AND NOT t."IsDeleted"
                               WHERE tm."TeamId" = rb."TeamId" AND tm."OrganizationId" = rb."OrganizationId"
                                 AND tm."UserId" = u."Id" AND NOT tm."IsDeleted")))::text[] AS "Bindings"
FROM "Users" u
JOIN "Memberships" m ON m."UserId" = u."Id" AND m."OrganizationId" = @organization_id AND NOT m."IsDeleted"
JOIN "Organizations" o ON o."Id" = m."OrganizationId" AND NOT o."IsDeleted"
LEFT JOIN "NotificationMutes" mu ON mu."UserId" = u."Id" AND NOT mu."IsDeleted"
WHERE NOT u."IsDeleted" AND NOT u."Disabled"
  AND (@in_app::boolean OR (u."Email" <> '' AND COALESCE(NOT mu."Muted", true)))
ORDER BY u."Id";

-- name: CountRecentChannelEmails :one
SELECT count(*)::bigint FROM "NotificationOutbox"
WHERE "RecipientUserId" = @user_id AND "Kind" = 'email' AND "EventName" <> 'notification.test' AND "NextAttemptAt" > @since::timestamptz;

-- name: GetNotificationUser :one
SELECT "Id", "Email", "Name", "Locale" FROM "Users" WHERE "Id" = @id AND NOT "IsDeleted" AND NOT "Disabled";

-- name: UpsertNotificationMute :exec
INSERT INTO "NotificationMutes" ("UserId", "Muted") VALUES (@user_id, @muted)
ON CONFLICT ("UserId") DO UPDATE SET "Muted" = EXCLUDED."Muted", "IsDeleted" = false, "UpdatedAt" = now();

-- name: GetNotificationMute :one
SELECT "Muted" FROM "NotificationMutes" WHERE "UserId" = @user_id AND NOT "IsDeleted";

-- name: NotificationProjectContext :one
SELECT p."Name" AS "ProjectName", p."Slug" AS "ProjectSlug" FROM "Projects" p
WHERE p."Id" = @project_id AND p."OrganizationId" = @organization_id;

-- name: NotificationEnvironmentName :one
SELECT "Name" FROM "Environments" WHERE "Id" = @id AND "OrganizationId" = @organization_id;

-- name: NotificationRunContext :one
SELECT "Number", "Ref" FROM "PipelineRuns" WHERE "Id" = @id AND "OrganizationId" = @organization_id;

-- name: NotificationDeploymentContext :one
SELECT d."EnvironmentId", d."RunId", d."RunNumber", d."Ref", e."ProjectId", e."Name" AS "EnvironmentName"
FROM "Deployments" d
JOIN "Environments" e ON e."Id" = d."EnvironmentId" AND e."OrganizationId" = d."OrganizationId"
WHERE d."Id" = @id AND d."OrganizationId" = @organization_id;

-- name: NotificationOutboxByScope :many
SELECT o.*, c."Name" AS "ChannelName", c."Kind" AS "ChannelKind"
FROM "NotificationOutbox" o
JOIN "NotificationChannels" c ON c."Id" = o."ChannelId" AND c."OrganizationId" = o."OrganizationId"
WHERE o."OrganizationId" = @organization_id AND c."ProjectId" IS NOT DISTINCT FROM sqlc.narg('project_id')::uuid
  AND NOT o."IsDeleted" AND (sqlc.narg('state')::text IS NULL OR o."State" = sqlc.narg('state')::text)
ORDER BY o."CreatedAt" DESC, o."Id" DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountNotificationOutboxByScope :one
SELECT count(*)
FROM "NotificationOutbox" o
JOIN "NotificationChannels" c ON c."Id" = o."ChannelId" AND c."OrganizationId" = o."OrganizationId"
WHERE o."OrganizationId" = @organization_id AND c."ProjectId" IS NOT DISTINCT FROM sqlc.narg('project_id')::uuid
  AND NOT o."IsDeleted" AND (sqlc.narg('state')::text IS NULL OR o."State" = sqlc.narg('state')::text);

-- name: GetNotificationDelivery :one
SELECT o.*, c."ProjectId" AS "ChannelProjectId", c."Enabled" AS "ChannelEnabled"
FROM "NotificationOutbox" o
JOIN "NotificationChannels" c ON c."Id" = o."ChannelId" AND c."OrganizationId" = o."OrganizationId"
WHERE o."Id" = @id AND o."OrganizationId" = @organization_id AND c."ProjectId" IS NOT DISTINCT FROM sqlc.narg('project_id')::uuid
  AND NOT o."IsDeleted"
FOR UPDATE OF o;

-- name: CountNotificationTestsSince :one
SELECT count(*) FROM "NotificationOutbox"
WHERE "ChannelId" = @channel_id AND "EventName" = 'notification.test' AND "CreatedAt" > @since::timestamptz;

-- name: TouchNotificationDevice :one
INSERT INTO "NotificationDevices" ("UserId", "Fingerprint", "LastSeenAt") VALUES (@user_id, @fingerprint, @now::timestamptz)
ON CONFLICT ("UserId", "Fingerprint") DO UPDATE SET "LastSeenAt" = EXCLUDED."LastSeenAt", "UpdatedAt" = now()
RETURNING (xmax = 0) AS "Inserted";

-- name: CountNotificationDevices :one
SELECT count(*) FROM "NotificationDevices" WHERE "UserId" = @user_id AND NOT "IsDeleted";

-- name: ListOfflineRunners :many
SELECT r."Id", r."OrganizationId", r."Name"
FROM "Runners" r
WHERE r."State" = 'active' AND NOT r."IsDeleted" AND r."LastSeenAt" IS NOT NULL AND r."LastSeenAt" <= @before::timestamptz
  AND NOT EXISTS (SELECT 1 FROM "NotificationRunnerOutages" o WHERE o."RunnerId" = r."Id" AND o."ResolvedAt" IS NULL)
ORDER BY r."Id"
LIMIT 100;

-- name: OpenRunnerOutage :execrows
INSERT INTO "NotificationRunnerOutages" ("OrganizationId", "RunnerId") VALUES (@organization_id, @runner_id)
ON CONFLICT DO NOTHING;

-- name: ResolveRunnerOutages :execrows
UPDATE "NotificationRunnerOutages" o
SET "ResolvedAt" = @now::timestamptz, "UpdatedAt" = now()
FROM "Runners" r
WHERE o."RunnerId" = r."Id" AND o."ResolvedAt" IS NULL AND (r."State" <> 'active' OR r."IsDeleted" OR r."LastSeenAt" > @before::timestamptz);

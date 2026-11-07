-- name: InsertOutbox :execrows
INSERT INTO "NotificationOutbox" (
    "OrganizationId", "ProjectId", "ChannelId", "RecipientUserId", "Kind", "EventId", "EventName", "Payload", "NextAttemptAt"
)
VALUES (
    sqlc.narg('organization_id'), @project_id, @channel_id, @recipient_user_id, @kind, @event_id, @event_name, @payload, @next_attempt_at
)
ON CONFLICT DO NOTHING;

-- name: ClaimOutbox :one
WITH picked AS (
    SELECT o."Id"
    FROM "NotificationOutbox" o
    WHERE NOT o."IsDeleted"
      AND ((o."State" = 'pending' AND o."NextAttemptAt" <= @now::timestamptz)
           OR (o."State" = 'sending' AND o."ClaimedUntil" <= @now::timestamptz))
      AND ((@internal::boolean AND o."Kind" = 'internal-email')
           OR (NOT @internal::boolean AND o."Kind" <> 'internal-email'
               AND (o."ChannelId" IS NULL OR o."ChannelId" <> ALL (@busy::uuid[]))
               AND (o."OrganizationId" IS NULL OR o."OrganizationId" <> ALL (@busy_orgs::uuid[]))))
    ORDER BY o."NextAttemptAt", o."Id"
    LIMIT 1
    FOR UPDATE SKIP LOCKED
)
UPDATE "NotificationOutbox" o
SET "State" = 'sending', "Attempts" = o."Attempts" + 1, "ClaimedUntil" = @lease_until::timestamptz,
    "LastAttemptAt" = @now::timestamptz, "UpdatedAt" = @now::timestamptz
FROM picked
WHERE o."Id" = picked."Id"
RETURNING o.*;

-- name: CompleteOutbox :execrows
UPDATE "NotificationOutbox"
SET "State" = 'sent', "SentAt" = @now::timestamptz, "ClaimedUntil" = NULL, "LastError" = '',
    "LastStatus" = sqlc.narg('status')::integer, "UpdatedAt" = @now::timestamptz
WHERE "Id" = @id AND "State" = 'sending' AND "Attempts" = @attempts::integer;

-- name: RescheduleOutbox :execrows
UPDATE "NotificationOutbox"
SET "State" = 'pending', "NextAttemptAt" = @next_attempt_at::timestamptz, "ClaimedUntil" = NULL, "LastError" = @last_error::text,
    "LastStatus" = sqlc.narg('status')::integer, "UpdatedAt" = @now::timestamptz
WHERE "Id" = @id AND "State" = 'sending' AND "Attempts" = @attempts::integer;

-- name: KillOutbox :execrows
UPDATE "NotificationOutbox"
SET "State" = 'dead', "ClaimedUntil" = NULL, "LastError" = @last_error::text,
    "LastStatus" = sqlc.narg('status')::integer, "UpdatedAt" = @now::timestamptz
WHERE "Id" = @id AND "State" = 'sending' AND "Attempts" = @attempts::integer;

-- name: RequeueOutbox :execrows
UPDATE "NotificationOutbox"
SET "State" = 'pending', "Attempts" = 0, "NextAttemptAt" = @now::timestamptz, "ClaimedUntil" = NULL, "LastError" = '',
    "LastStatus" = NULL, "UpdatedAt" = @now::timestamptz
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND "State" = 'dead' AND NOT "IsDeleted";

-- name: TryOutboxLock :one
SELECT pg_try_advisory_lock(@lock_key::bigint) AS "Locked";

-- name: ReleaseOutboxLock :one
SELECT pg_advisory_unlock(@lock_key::bigint) AS "Released";

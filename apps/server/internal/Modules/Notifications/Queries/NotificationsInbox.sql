-- name: InsertNotificationInbox :execrows
INSERT INTO "NotificationInbox" ("UserId", "OrganizationId", "EventId", "EventName", "TitleKey", "Params", "Link", "CreatedAt")
VALUES (@user_id, sqlc.narg('organization_id'), @event_id, @event_name, @title_key, @params, @link, @created_at)
ON CONFLICT ("UserId", "EventId") DO NOTHING;

-- name: TrimNotificationInbox :execrows
UPDATE "NotificationInbox" SET "IsDeleted" = true, "UpdatedAt" = now()
WHERE "Id" IN (
    SELECT n."Id" FROM "NotificationInbox" n
    WHERE n."UserId" = @user_id AND NOT n."IsDeleted"
    ORDER BY n."CreatedAt" DESC, n."Id" DESC
    OFFSET @keep::integer
);

-- name: PurgeNotificationInbox :execrows
UPDATE "NotificationInbox" SET "IsDeleted" = true, "UpdatedAt" = now()
WHERE NOT "IsDeleted" AND "CreatedAt" < @before::timestamptz;

-- name: CountNotificationInbox :one
SELECT count(*)::bigint FROM "NotificationInbox" n
WHERE n."UserId" = @user_id AND NOT n."IsDeleted"
  AND (NOT @unread_only::boolean OR NOT n."IsRead")
  AND (n."OrganizationId" IS NULL OR EXISTS (
        SELECT 1 FROM "Memberships" m
        WHERE m."UserId" = n."UserId" AND m."OrganizationId" = n."OrganizationId" AND NOT m."IsDeleted"));

-- name: ListNotificationInbox :many
SELECT n."Id", n."CreatedAt", n."OrganizationId", n."EventId", n."EventName", n."TitleKey", n."Params", n."Link", n."IsRead", n."ReadAt"
FROM "NotificationInbox" n
WHERE n."UserId" = @user_id AND NOT n."IsDeleted"
  AND (NOT @unread_only::boolean OR NOT n."IsRead")
  AND (n."OrganizationId" IS NULL OR EXISTS (
        SELECT 1 FROM "Memberships" m
        WHERE m."UserId" = n."UserId" AND m."OrganizationId" = n."OrganizationId" AND NOT m."IsDeleted"))
ORDER BY n."CreatedAt" DESC, n."Id" DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: MarkNotificationInboxRead :one
UPDATE "NotificationInbox" SET "IsRead" = true, "ReadAt" = COALESCE("ReadAt", @now::timestamptz), "UpdatedAt" = now()
WHERE "Id" = @id AND "UserId" = @user_id AND NOT "IsDeleted"
RETURNING "Id", "CreatedAt", "OrganizationId", "EventId", "EventName", "TitleKey", "Params", "Link", "IsRead", "ReadAt";

-- name: MarkAllNotificationInboxRead :execrows
UPDATE "NotificationInbox" SET "IsRead" = true, "ReadAt" = @now::timestamptz, "UpdatedAt" = now()
WHERE "UserId" = @user_id AND NOT "IsDeleted" AND NOT "IsRead";

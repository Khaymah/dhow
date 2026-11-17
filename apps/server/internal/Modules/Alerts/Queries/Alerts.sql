-- name: GetAlertRecipient :one
SELECT "Email", "Name", "Locale"
FROM "Users"
WHERE "Id" = @id AND NOT "IsDeleted" AND NOT "Disabled";

-- name: DropOldestPendingAlerts :execrows
UPDATE "NotificationOutbox" AS target
SET "IsDeleted" = true, "UpdatedAt" = now()
WHERE target."Id" IN (
    SELECT o."Id" FROM "NotificationOutbox" AS o
    WHERE o."Kind" = 'internal-email' AND o."RecipientUserId" = @user_id AND o."State" = 'pending' AND NOT o."IsDeleted"
      AND o."EventName" NOT LIKE 'account.%' AND o."EventName" NOT LIKE 'notification.%'
    ORDER BY o."CreatedAt" DESC, o."Id" DESC
    OFFSET @keep::int
);

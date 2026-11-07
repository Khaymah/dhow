-- name: GetAlertRecipient :one
SELECT "Email", "Name", "Locale"
FROM "Users"
WHERE "Id" = @id AND NOT "IsDeleted" AND NOT "Disabled";

-- name: CountPendingInternalEmails :one
SELECT count(*)::bigint FROM "NotificationOutbox"
WHERE "Kind" = 'internal-email' AND "State" IN ('pending', 'sending') AND NOT "IsDeleted";

-- name: LockInstance :one
SELECT "SetupCompletedAt"
FROM "Instance"
WHERE "Id" = 1
FOR UPDATE;

-- name: CompleteInstanceSetup :exec
UPDATE "Instance"
SET "SetupCompletedAt" = @completed_at::timestamptz, "UpdatedAt" = @completed_at
WHERE "Id" = 1;

-- name: InvalidateSetupTokens :exec
UPDATE "SetupTokens"
SET "IsDeleted" = true, "UpdatedAt" = @now
WHERE NOT "IsDeleted" AND "UsedAt" IS NULL;

-- name: InsertSetupToken :exec
INSERT INTO "SetupTokens" ("TokenHash", "ExpiresAt", "CreatedAt", "UpdatedAt")
VALUES (@token_hash, @expires_at, @now, @now);

-- name: GetUsableSetupToken :one
SELECT "Id", "ExpiresAt"
FROM "SetupTokens"
WHERE "TokenHash" = @token_hash AND NOT "IsDeleted" AND "UsedAt" IS NULL AND "ExpiresAt" > @now;

-- name: LockUsableSetupToken :one
SELECT "Id"
FROM "SetupTokens"
WHERE "TokenHash" = @token_hash AND NOT "IsDeleted" AND "UsedAt" IS NULL AND "ExpiresAt" > @now
FOR UPDATE;

-- name: MarkSetupTokenUsed :exec
UPDATE "SetupTokens"
SET "UsedAt" = @now::timestamptz, "UpdatedAt" = @now
WHERE "Id" = @id;

-- name: InsertOrganization :one
INSERT INTO "Organizations" ("Name", "Slug", "CreatedAt", "UpdatedAt")
VALUES (@name, @slug, @now, @now)
RETURNING "Id", "Name", "Slug", "CreatedAt", "RequireSecondFactor", "RequireSecondFactorSince";

-- name: InsertUser :one
INSERT INTO "Users" ("Email", "Name", "PasswordHash", "PasswordChangedAt", "Locale", "CreatedAt", "UpdatedAt")
VALUES (@email, @name, @password_hash, @now, @locale, @now, @now)
RETURNING "Id", "Email", "Name", "Locale";

-- name: InsertMembership :exec
INSERT INTO "Memberships" ("UserId", "OrganizationId", "Role", "CreatedAt", "UpdatedAt")
VALUES (@user_id, @organization_id, @role, @now, @now);

-- name: GetOrganizationForMember :one
SELECT o."Id", o."Name", o."Slug", o."CreatedAt", o."RequireSecondFactor", o."RequireSecondFactorSince", o."LogRetentionDays"
FROM "Organizations" o
JOIN "Memberships" m ON m."OrganizationId" = o."Id" AND m."UserId" = @user_id AND NOT m."IsDeleted"
WHERE o."Id" = @id AND NOT o."IsDeleted";

-- name: UpdateOrganizationName :one
UPDATE "Organizations" o
SET "Name" = @name, "UpdatedAt" = @now
WHERE o."Id" = @id AND NOT o."IsDeleted"
  AND EXISTS (SELECT 1 FROM "Memberships" m WHERE m."OrganizationId" = o."Id" AND m."UserId" = @user_id AND NOT m."IsDeleted")
RETURNING o."Id", o."Name", o."Slug", o."CreatedAt", o."RequireSecondFactor", o."RequireSecondFactorSince";

-- name: SetOrganizationSecondFactor :one
UPDATE "Organizations"
SET "RequireSecondFactor" = @rule, "RequireSecondFactorSince" = sqlc.narg(since)::timestamptz,
    "RequireSecondFactorAdminsSince" = sqlc.narg(admins_since)::timestamptz, "UpdatedAt" = @now
WHERE "Id" = @id AND NOT "IsDeleted"
RETURNING "Id", "Name", "Slug", "CreatedAt", "RequireSecondFactor", "RequireSecondFactorSince";

-- name: MarkInstanceOwner :exec
UPDATE "Users"
SET "IsInstanceOwner" = true, "UpdatedAt" = @now
WHERE "Id" = @id AND NOT "IsDeleted";

-- name: ListMembers :many
SELECT u."Id" AS "UserId", u."Name", u."Email", m."Role", m."CreatedAt" AS "JoinedAt",
       ARRAY(SELECT r."Role" FROM "OrganizationTeamRoles" r
             WHERE r."UserId" = m."UserId" AND r."OrganizationId" = m."OrganizationId")::text[] AS "TeamRoles",
       EXISTS (SELECT 1 FROM "TotpFactors" t
               WHERE t."UserId" = u."Id" AND t."ConfirmedAt" IS NOT NULL AND NOT t."IsDeleted") AS "HasTotp",
       EXISTS (SELECT 1 FROM "Passkeys" p WHERE p."UserId" = u."Id" AND NOT p."IsDeleted") AS "HasPasskey",
       u."MfaResetAt",
       COALESCE(teams."Ids", '{}')::uuid[] AS "TeamIds",
       COALESCE(teams."Names", '{}')::text[] AS "TeamNames",
       m."LastSeenAt" AS "LastActiveAt"
FROM "Memberships" m
JOIN "Users" u ON u."Id" = m."UserId" AND NOT u."IsDeleted"
LEFT JOIN LATERAL (
    SELECT array_agg(t."Id" ORDER BY lower(t."Name"), t."Id") AS "Ids",
           array_agg(t."Name" ORDER BY lower(t."Name"), t."Id") AS "Names"
    FROM "TeamMembers" tm
    JOIN "Teams" t ON t."Id" = tm."TeamId" AND t."OrganizationId" = tm."OrganizationId" AND NOT t."IsDeleted"
    WHERE tm."UserId" = m."UserId" AND tm."OrganizationId" = m."OrganizationId" AND NOT tm."IsDeleted"
) teams ON true
WHERE m."OrganizationId" = @organization_id AND NOT m."IsDeleted"
ORDER BY m."CreatedAt", u."Id"
LIMIT @page_limit OFFSET @page_offset;

-- name: CountMembers :one
SELECT count(*)
FROM "Memberships" m
JOIN "Users" u ON u."Id" = m."UserId" AND NOT u."IsDeleted"
WHERE m."OrganizationId" = @organization_id AND NOT m."IsDeleted";

-- name: SetOrganizationLogRetention :one
UPDATE "Organizations"
SET "LogRetentionDays" = sqlc.narg(days)::integer, "UpdatedAt" = @now
WHERE "Id" = @id AND NOT "IsDeleted"
RETURNING "Id", "Name", "Slug", "CreatedAt", "RequireSecondFactor", "RequireSecondFactorSince", "LogRetentionDays";

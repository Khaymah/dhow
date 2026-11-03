-- name: LockAttemptLog :one
SELECT "Id", "JobId", "OrganizationId", "State", "LastSeq", "LogBytes", ("LogTruncatedAt" IS NOT NULL)::boolean AS "Truncated",
       ("LateResultLoggedAt" IS NOT NULL)::boolean AS "LateResultLogged", "LogMaxBytes"
FROM "JobAttempts"
WHERE "Id" = @id
FOR UPDATE;

-- name: InsertLogChunks :execrows
INSERT INTO "LogChunks" ("OrganizationId", "JobId", "AttemptId", "Seq", "Stream", "StepIndex", "EmittedAt", "Data")
SELECT @organization_id, @job_id, @attempt_id, c."Seq", c."Stream", NULLIF(c."StepIndex", -1),
       CASE WHEN c."HasEmittedAt" THEN c."EmittedAt" END, c."Data"
FROM (SELECT unnest(@seqs::bigint[]) AS "Seq", unnest(@streams::text[]) AS "Stream",
             unnest(@step_indexes::integer[]) AS "StepIndex", unnest(@has_emitted_at::boolean[]) AS "HasEmittedAt",
             unnest(@emitted_at::timestamptz[]) AS "EmittedAt", unnest(@data::bytea[]) AS "Data") c;

-- name: AdvanceAttemptLog :exec
UPDATE "JobAttempts"
SET "LastSeq" = @last_seq, "LogBytes" = @log_bytes,
    "LogTruncatedAt" = CASE WHEN @truncated::boolean THEN COALESCE("LogTruncatedAt", now()) ELSE "LogTruncatedAt" END,
    "UpdatedAt" = now()
WHERE "Id" = @id;

-- name: MarkLateResultLogged :exec
UPDATE "JobAttempts"
SET "LateResultLoggedAt" = now(), "LastSeq" = @last_seq, "LogBytes" = @log_bytes, "UpdatedAt" = now()
WHERE "Id" = @id AND "LateResultLoggedAt" IS NULL;

-- name: NotifyLogAppended :exec
SELECT pg_notify('dhow_log_appended', @payload::text);

-- name: ListenLogAppended :exec
LISTEN dhow_log_appended;

-- name: JobExists :one
SELECT EXISTS (SELECT 1
               FROM "Jobs"
               WHERE "Id" = @id AND "OrganizationId" = @organization_id AND NOT "IsDeleted")::boolean;

-- name: GetLogAttempt :one
SELECT "Id", "Number", "State", "LastSeq", ("LogTruncatedAt" IS NOT NULL)::boolean AS "Truncated", "LeasedAt",
       (SELECT o."LogRetentionDays" FROM "Organizations" o WHERE o."Id" = @organization_id) AS "OrgRetentionDays"
FROM "JobAttempts"
WHERE "JobId" = @job_id AND "OrganizationId" = @organization_id AND NOT "IsDeleted"
  AND (sqlc.narg(number)::integer IS NULL OR "Number" = sqlc.narg(number)::integer)
ORDER BY "Number" DESC
LIMIT 1;

-- name: GetLogAttemptState :one
SELECT "State", "LastSeq", ("LogTruncatedAt" IS NOT NULL)::boolean AS "Truncated"
FROM "JobAttempts"
WHERE "Id" = @id AND "OrganizationId" = @organization_id;

-- name: ListLogChunks :many
WITH page AS (
    SELECT c."Id", c."CreatedAt",
           sum(octet_length(c."Data")) OVER (ORDER BY c."Seq" ROWS UNBOUNDED PRECEDING) - octet_length(c."Data") AS "Before"
    FROM "LogChunks" c
    WHERE c."AttemptId" = @attempt_id AND c."OrganizationId" = @organization_id AND c."Seq" >= @from_seq
      AND c."CreatedAt" >= @since AND NOT c."IsDeleted"
    ORDER BY c."Seq"
    LIMIT @row_limit
)
SELECT l."Seq", l."Stream", l."StepIndex", l."EmittedAt", l."CreatedAt", l."Data"
FROM page p
JOIN "LogChunks" l ON l."Id" = p."Id" AND l."CreatedAt" = p."CreatedAt"
WHERE p."Before" < @max_bytes::bigint
ORDER BY l."Seq";

-- name: TryLockLogPartitions :one
SELECT pg_try_advisory_xact_lock(hashtextextended('dhow.log-partitions', 0))::boolean;

-- name: SetLocalLockTimeout :exec
SELECT set_config('lock_timeout', @timeout::text, true);

-- name: EnsureLogPartitions :one
SELECT "EnsureLogPartitions"(@days_ahead::integer)::integer;

-- name: DropLogPartitions :one
SELECT "DropLogPartitions"(@retention_days::integer)::integer;

-- name: LogPartitionReady :one
SELECT "LogPartitionReady"()::boolean;

-- name: UnreadableLogPartitions :one
SELECT "UnreadableLogPartitions"()::text[];

-- name: OrganizationsWithLogRetention :many
SELECT "Id", "LogRetentionDays"::integer AS "Days"
FROM "Organizations"
WHERE "LogRetentionDays" IS NOT NULL AND NOT "IsDeleted";

-- name: ScrubOrganizationLogs :execrows
UPDATE "LogChunks"
SET "IsDeleted" = true, "Data" = '\x00'::bytea, "UpdatedAt" = now()
WHERE ("Id", "CreatedAt") IN (SELECT l."Id", l."CreatedAt"
                              FROM "LogChunks" l
                              WHERE l."OrganizationId" = @organization_id AND l."CreatedAt" < @before AND NOT l."IsDeleted"
                              LIMIT @batch);

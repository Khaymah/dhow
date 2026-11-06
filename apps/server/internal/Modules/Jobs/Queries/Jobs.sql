-- name: InsertJob :one
INSERT INTO "Jobs" ("OrganizationId", "CreatedByUserId", "Name", "Spec", "SpecDigest", "Image", "Labels", "Isolation",
                    "Priority", "MaxInfraRetries", "QueueDeadline", "OriginKind", "OriginId", "RequiredFeatures")
VALUES (@organization_id, @created_by_user_id, @name, @spec, @spec_digest, @image, @labels::text[], @isolation, @priority,
        @max_infra_retries, now() + make_interval(secs => @queue_timeout_seconds::float8), @origin_kind,
        sqlc.narg('origin_id')::uuid, @required_features::text[])
RETURNING *;

-- name: NotifyJobsQueued :exec
SELECT pg_notify('dhow_jobs_queued', @organization_id::text);

-- name: CountJobs :one
SELECT count(*)
FROM "Jobs"
WHERE "OrganizationId" = @organization_id AND NOT "IsDeleted"
  AND (cardinality(@states::text[]) = 0 OR "State" = ANY(@states::text[]));

-- name: ListJobs :many
SELECT "Id", "CreatedAt", "OrganizationId", "CreatedByUserId", "Name", "SpecDigest", "Image", "Labels", "Isolation",
       "Priority", "State", "FailureReason", "QueuedAt", "StartedAt", "FinishedAt", "Deadline", "QueueDeadline",
       "CancelRequestedAt", "AttemptCount", "MaxInfraRetries"
FROM "Jobs"
WHERE "OrganizationId" = @organization_id AND NOT "IsDeleted"
  AND (cardinality(@states::text[]) = 0 OR "State" = ANY(@states::text[]))
ORDER BY "CreatedAt" DESC, "Id" DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: LockOrganizationQueue :exec
SELECT pg_advisory_xact_lock(hashtextextended('dhow.organization-queue:' || @organization_id::text, 0));

-- name: CountQueuedJobs :one
SELECT count(*)
FROM "Jobs"
WHERE "OrganizationId" = @organization_id AND "State" = 'queued' AND NOT "IsDeleted"
  AND (cardinality("RequiredFeatures") > 0) = @needs_features::boolean;

-- name: IsEnabledMemberOf :one
SELECT EXISTS (SELECT 1
               FROM "Memberships" m
               JOIN "Users" u ON u."Id" = m."UserId" AND NOT u."IsDeleted" AND NOT u."Disabled"
               WHERE m."UserId" = @user_id AND m."OrganizationId" = @organization_id AND NOT m."IsDeleted")::boolean;

-- name: OriginUsedElsewhere :one
SELECT EXISTS (SELECT 1
               FROM "Jobs"
               WHERE "OriginId" = @origin_id AND "OrganizationId" <> @organization_id)::boolean;

-- name: GetJob :one
SELECT *
FROM "Jobs"
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND NOT "IsDeleted";

-- name: ListJobAttempts :many
SELECT *
FROM "JobAttempts"
WHERE "JobId" = @job_id AND "OrganizationId" = @organization_id AND NOT "IsDeleted"
ORDER BY "Number";

-- name: ListJobSteps :many
SELECT s.*
FROM "AttemptSteps" s
JOIN "JobAttempts" a ON a."Id" = s."AttemptId"
WHERE a."JobId" = @job_id AND a."OrganizationId" = @organization_id AND NOT s."IsDeleted"
ORDER BY a."Number", s."Index";

-- name: CountMatchingRunners :one
SELECT count(*)
FROM "Runners" r
WHERE r."OrganizationId" = @organization_id AND r."State" = 'active' AND NOT r."IsDeleted"
  AND @labels::text[] <@ (r."Labels" || r."CapabilityLabels") AND @isolation::text = ANY (r."IsolationLevels");

-- name: LockJob :one
SELECT *
FROM "Jobs"
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND NOT "IsDeleted"
FOR UPDATE;

-- name: CancelQueuedJob :one
UPDATE "Jobs"
SET "State" = 'canceled', "FailureReason" = 'CANCELED', "FinishedAt" = now(), "CancelRequestedAt" = now(),
    "CancelRequestedBy" = sqlc.narg('user_id')::uuid, "UpdatedAt" = now()
WHERE "Id" = @id AND "State" = 'queued'
RETURNING *;

-- name: RequestJobCancel :one
UPDATE "Jobs"
SET "CancelRequestedAt" = now(), "CancelRequestedBy" = sqlc.narg('user_id')::uuid, "UpdatedAt" = now()
WHERE "Id" = @id AND "State" IN ('leased', 'running') AND "CancelRequestedAt" IS NULL
RETURNING *;

-- name: LockRunnerSlots :exec
SELECT pg_advisory_xact_lock(hashtextextended('dhow.runner-slots:' || @runner_id::text, 0));

-- name: RunnerSlots :one
SELECT now()::timestamptz AS "Now",
       (SELECT count(*)
        FROM "Leases" l
        WHERE l."RunnerId" = @runner_id AND l."State" = 'active' AND l."ExpiresAt" > now() AND NOT l."IsDeleted")::bigint AS "ActiveLeases",
       EXISTS (SELECT 1
               FROM "Runners" r
               WHERE r."Id" = @runner_id AND r."State" = 'active' AND NOT r."IsDeleted")::boolean AS "RunnerActive";

-- name: ClaimQueuedJob :one
SELECT *
FROM "Jobs" j
WHERE j."OrganizationId" = @organization_id AND j."State" = 'queued' AND NOT j."IsDeleted"
  AND j."Labels" <@ @labels::text[] AND j."Isolation" = ANY (@isolation_levels::text[])
  AND j."RequiredFeatures" <@ @features::text[]
  AND j."QueueDeadline" > now() AND j."CancelRequestedAt" IS NULL
  AND NOT EXISTS (SELECT 1
                  FROM "JobAttempts" a
                  WHERE a."JobId" = j."Id" AND a."RunnerId" = @runner_id
                    AND a."FailureReason" = 'RESOURCE_REQUEST_EXCEEDS_RUNNER')
ORDER BY j."Priority" DESC, j."QueuedAt", j."Id"
LIMIT 1
FOR UPDATE SKIP LOCKED;

-- name: NextFencingToken :one
SELECT nextval('"FencingTokens"')::bigint;

-- name: InsertAttempt :one
INSERT INTO "JobAttempts" ("Id", "JobId", "OrganizationId", "RunnerId", "Number", "FencingToken", "LogMaxBytes")
VALUES (@id, @job_id, @organization_id, @runner_id, @number, @fencing_token, @log_max_bytes)
RETURNING *;

-- name: InsertLease :one
INSERT INTO "Leases" ("Id", "AttemptId", "JobId", "OrganizationId", "RunnerId", "FencingToken", "GrantDigest", "ExpiresAt")
VALUES (@id, @attempt_id, @job_id, @organization_id, @runner_id, @fencing_token, @grant_digest,
        now() + make_interval(secs => @ttl_seconds::float8))
RETURNING *;

-- name: MarkJobLeased :one
UPDATE "Jobs"
SET "State" = 'leased', "AttemptCount" = "AttemptCount" + 1, "UpdatedAt" = now()
WHERE "Id" = @id AND "State" = 'queued'
RETURNING "AttemptCount";

-- name: LockActiveLease :one
SELECT l."Id", l."AttemptId", l."JobId", l."OrganizationId", l."RunnerId", l."FencingToken", l."GrantDigest",
       a."State" AS "AttemptState", a."MutationStartedAt", j."CancelRequestedAt", j."Spec"
FROM "Leases" l
JOIN "JobAttempts" a ON a."Id" = l."AttemptId"
JOIN "Jobs" j ON j."Id" = l."JobId"
WHERE l."Id" = @lease_id AND l."FencingToken" = @fencing_token AND l."RunnerId" = @runner_id
  AND l."State" = 'active' AND l."ExpiresAt" > now() AND NOT l."IsDeleted"
FOR UPDATE OF l;

-- name: LockLeaseForCompletion :one
SELECT l."Id", l."AttemptId", l."JobId", l."State", (l."ExpiresAt" > now())::boolean AS "Live",
       a."State" AS "AttemptState", a."CompletionDigest", j."Spec"
FROM "Leases" l
JOIN "JobAttempts" a ON a."Id" = l."AttemptId"
JOIN "Jobs" j ON j."Id" = l."JobId"
WHERE l."Id" = @lease_id AND l."FencingToken" = @fencing_token AND l."RunnerId" = @runner_id AND NOT l."IsDeleted"
FOR UPDATE OF l;

-- name: JobCancelRequestedAt :one
SELECT "CancelRequestedAt"
FROM "Jobs"
WHERE "Id" = @id
FOR SHARE;

-- name: ExtendLease :one
UPDATE "Leases"
SET "ExpiresAt" = now() + make_interval(secs => @ttl_seconds::float8), "LastHeartbeatAt" = now(), "UpdatedAt" = now()
WHERE "Id" = @id AND "State" = 'active'
RETURNING "ExpiresAt";

-- name: StartAttempt :one
UPDATE "JobAttempts"
SET "State" = 'running', "StartedAt" = now(), "UpdatedAt" = now()
WHERE "Id" = @id AND "State" = 'leased'
RETURNING *;

-- name: MarkJobRunning :one
UPDATE "Jobs"
SET "State" = 'running', "StartedAt" = COALESCE("StartedAt", now()),
    "Deadline" = now() + make_interval(secs => @timeout_seconds::float8), "UpdatedAt" = now()
WHERE "Id" = @id AND "State" = 'leased'
RETURNING *;

-- name: InsertAttemptStep :one
INSERT INTO "AttemptSteps" ("AttemptId", "Index", "Name", "Mutating")
VALUES (@attempt_id, @index, @name, @mutating)
ON CONFLICT ("AttemptId", "Index") DO NOTHING
RETURNING "Id";

-- name: MarkMutationStarted :exec
UPDATE "JobAttempts"
SET "MutationStartedAt" = COALESCE("MutationStartedAt", now()), "UpdatedAt" = now()
WHERE "Id" = @id AND "State" = 'running';

-- name: GetAttemptStepState :one
SELECT "State"
FROM "AttemptSteps"
WHERE "AttemptId" = @attempt_id AND "Index" = @index AND NOT "IsDeleted";

-- name: EndAttemptStep :execrows
UPDATE "AttemptSteps"
SET "State" = @state, "ExitCode" = @exit_code, "FailureReason" = @failure_reason, "FinishedAt" = now(), "UpdatedAt" = now()
WHERE "AttemptId" = @attempt_id AND "Index" = @index AND "State" = 'running';

-- name: FinishAttempt :one
UPDATE "JobAttempts"
SET "State" = @state, "FailureReason" = @failure_reason, "ExitCode" = sqlc.narg(exit_code)::integer,
    "CompletionDigest" = sqlc.narg(completion_digest)::bytea, "FinishedAt" = now(), "UpdatedAt" = now()
WHERE "Id" = @id AND "State" IN ('leased', 'running')
RETURNING *;

-- name: EndLease :execrows
UPDATE "Leases"
SET "State" = @state, "EndedAt" = now(), "UpdatedAt" = now()
WHERE "Id" = @id AND "State" = 'active';

-- name: InterruptAttemptSteps :exec
UPDATE "AttemptSteps"
SET "State" = 'interrupted', "FinishedAt" = now(), "UpdatedAt" = now()
WHERE "AttemptId" = @attempt_id AND "State" = 'running';

-- name: LockJobByID :one
SELECT *
FROM "Jobs"
WHERE "Id" = @id
FOR UPDATE;

-- name: FinishJob :one
UPDATE "Jobs"
SET "State" = @state, "FailureReason" = @failure_reason, "FinishedAt" = now(), "UpdatedAt" = now()
WHERE "Id" = @id AND "State" = @from_state
RETURNING *;

-- name: RequeueJob :one
UPDATE "Jobs"
SET "State" = 'queued', "Deadline" = NULL, "UpdatedAt" = now()
WHERE "Id" = @id AND "State" = @from_state
RETURNING *;

-- name: PickExpiredLease :one
SELECT "Id"
FROM "Leases"
WHERE "State" = 'active' AND "ExpiresAt" < now() - make_interval(secs => @grace_seconds::float8) AND NOT "IsDeleted"
ORDER BY "ExpiresAt"
LIMIT 1
FOR UPDATE SKIP LOCKED;

-- name: PickRevokedRunnerLease :one
SELECT l."Id"
FROM "Leases" l
JOIN "Runners" r ON r."Id" = l."RunnerId"
WHERE l."State" = 'active' AND r."State" = 'revoked' AND NOT l."IsDeleted"
ORDER BY l."ExpiresAt"
LIMIT 1
FOR UPDATE OF l SKIP LOCKED;

-- name: PickOverdueLease :one
SELECT l."Id"
FROM "Leases" l
JOIN "Jobs" j ON j."Id" = l."JobId"
WHERE l."State" = 'active' AND NOT l."IsDeleted" AND j."Deadline" IS NOT NULL
  AND j."Deadline" < now() - make_interval(secs => @grace_seconds::float8)
ORDER BY j."Deadline"
LIMIT 1
FOR UPDATE OF l SKIP LOCKED;

-- name: PickUnstartedLease :one
SELECT l."Id"
FROM "Leases" l
JOIN "JobAttempts" a ON a."Id" = l."AttemptId"
WHERE l."State" = 'active' AND NOT l."IsDeleted" AND a."State" = 'leased'
  AND a."LeasedAt" < now() - make_interval(secs => @start_seconds::float8)
ORDER BY a."LeasedAt"
LIMIT 1
FOR UPDATE OF l SKIP LOCKED;

-- name: PickCanceledUnstartedLease :one
SELECT l."Id"
FROM "Leases" l
JOIN "JobAttempts" a ON a."Id" = l."AttemptId"
JOIN "Jobs" j ON j."Id" = l."JobId"
WHERE l."State" = 'active' AND NOT l."IsDeleted" AND a."State" = 'leased'
  AND j."CancelRequestedAt" < now() - make_interval(secs => @grace_seconds::float8)
ORDER BY j."CancelRequestedAt"
LIMIT 1
FOR UPDATE OF l SKIP LOCKED;

-- name: LockRunnerLease :one
SELECT "Id"
FROM "Leases"
WHERE "RunnerId" = @runner_id AND "State" = 'active' AND NOT "IsDeleted"
ORDER BY "CreatedAt"
LIMIT 1
FOR UPDATE;

-- name: GetLeaseAttempt :one
SELECT l."Id", l."AttemptId", l."JobId", a."State" AS "AttemptState"
FROM "Leases" l
JOIN "JobAttempts" a ON a."Id" = l."AttemptId"
WHERE l."Id" = @id;

-- name: FailTimedOutQueuedJobs :many
UPDATE "Jobs"
SET "State" = 'failed', "FailureReason" = 'QUEUE_TIMEOUT', "FinishedAt" = now(), "UpdatedAt" = now()
WHERE "Id" IN (SELECT q."Id"
               FROM "Jobs" q
               WHERE q."State" = 'queued' AND q."QueueDeadline" < now() AND NOT q."IsDeleted"
               ORDER BY q."QueueDeadline"
               LIMIT @batch
               FOR UPDATE SKIP LOCKED)
RETURNING *;

-- name: ListenJobsQueued :exec
LISTEN dhow_jobs_queued;

-- name: GetJobProject :one
SELECT r."ProjectId"
FROM "Jobs" j
JOIN "RunJobs" rj ON rj."Id" = j."OriginId" AND rj."OrganizationId" = j."OrganizationId"
JOIN "PipelineRuns" r ON r."Id" = rj."RunId" AND r."OrganizationId" = rj."OrganizationId"
WHERE j."Id" = @id AND j."OrganizationId" = @organization_id AND j."OriginKind" = 'pipeline';

-- name: GetJobRun :one
SELECT p."Id" AS "ProjectId", p."Name" AS "ProjectName", r."Id" AS "RunId", r."Number" AS "RunNumber", rj."Kind"
FROM "Jobs" j
JOIN "RunJobs" rj ON rj."Id" = j."OriginId" AND rj."OrganizationId" = j."OrganizationId"
JOIN "PipelineRuns" r ON r."Id" = rj."RunId" AND r."OrganizationId" = rj."OrganizationId"
JOIN "Projects" p ON p."Id" = r."ProjectId" AND p."OrganizationId" = r."OrganizationId"
WHERE j."Id" = @job_id AND j."OrganizationId" = @organization_id AND j."OriginKind" = 'pipeline'
  AND NOT j."IsDeleted" AND NOT r."IsDeleted" AND NOT p."IsDeleted";

-- name: GetRunnerNames :many
SELECT "Id", "Name"
FROM "Runners"
WHERE "OrganizationId" = @organization_id AND "Id" = ANY(@ids::uuid[]);

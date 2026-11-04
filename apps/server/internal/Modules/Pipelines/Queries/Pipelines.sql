-- name: GetRunProject :one
SELECT p."Id", p."OrganizationId", p."Name", p."State", p."PipelinePath", p."ForkPolicy",
       r."Id" AS "RepositoryId", r."Provider", r."DefaultBranch"
FROM "Projects" p
LEFT JOIN "Repositories" r ON r."ProjectId" = p."Id" AND r."OrganizationId" = p."OrganizationId" AND NOT r."IsDeleted"
WHERE p."Id" = @project_id AND p."OrganizationId" = @organization_id AND NOT p."IsDeleted";

-- name: RunCreatorIsMember :one
SELECT EXISTS (SELECT 1
               FROM "Memberships" m
               JOIN "Users" u ON u."Id" = m."UserId" AND NOT u."IsDeleted" AND NOT u."Disabled"
               WHERE m."UserId" = @user_id AND m."OrganizationId" = @organization_id AND NOT m."IsDeleted")::boolean;

-- name: LockRunTrigger :exec
SELECT pg_advisory_xact_lock(hashtextextended('dhow.run-trigger:' || (@repository_id::uuid)::text || ':' || @ref::text, 0));

-- name: FindCommitRun :one
SELECT "Id"
FROM "PipelineRuns"
WHERE "RepositoryId" = @repository_id AND "Ref" = @ref AND "Sha" = @sha AND "Trigger" = @trigger AND "IsFork" = @is_fork;

-- name: TrustedShaOnRef :one
SELECT EXISTS (SELECT 1
               FROM "PipelineRuns"
               WHERE "RepositoryId" = @repository_id AND "Ref" = @ref AND "Sha" = @sha AND "Trigger" IN ('push', 'tag')
                 AND NOT "IsFork")::boolean;

-- name: LatestTrustedSha :one
SELECT "Sha"::text
FROM "PipelineRuns"
WHERE "RepositoryId" = @repository_id AND "Ref" = @ref AND "Trigger" IN ('push', 'tag') AND NOT "IsFork" AND "Sha" IS NOT NULL
ORDER BY "CreatedAt" DESC, "Number" DESC
LIMIT 1;

-- name: NextRunNumber :one
INSERT INTO "PipelineRunCounters" ("ProjectId", "Next")
VALUES (@project_id, 2)
ON CONFLICT ("ProjectId") DO UPDATE SET "Next" = "PipelineRunCounters"."Next" + 1, "UpdatedAt" = now()
RETURNING ("Next" - 1)::bigint AS "Number";

-- name: InsertRun :one
INSERT INTO "PipelineRuns" ("OrganizationId", "ProjectId", "RepositoryId", "Provider", "Number", "Trigger", "Ref", "BaseRef", "Sha",
                            "BaseSha", "IsFork", "PrNumber", "Actor", "Inputs", "DefinitionPath", "DefinitionSha", "State",
                            "DeliveryId", "TriggeredByUserId")
VALUES (@organization_id, @project_id, sqlc.narg('repository_id')::uuid, sqlc.narg('provider')::text, @number, @trigger, @ref,
        sqlc.narg('base_ref')::text, sqlc.narg('sha')::text, sqlc.narg('base_sha')::text, @is_fork, sqlc.narg('pr_number')::integer,
        sqlc.narg('actor')::text, @inputs, @definition_path, sqlc.narg('definition_sha')::text, @state,
        sqlc.narg('delivery_id')::uuid, sqlc.narg('triggered_by_user_id')::uuid)
RETURNING *;

-- name: GetRun :one
SELECT *
FROM "PipelineRuns"
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND "ProjectId" = @project_id AND NOT "IsDeleted";

-- name: LockRun :one
SELECT *
FROM "PipelineRuns"
WHERE "Id" = @id
FOR UPDATE;

-- name: CountRuns :one
SELECT count(*)
FROM "PipelineRuns"
WHERE "ProjectId" = @project_id AND "OrganizationId" = @organization_id AND NOT "IsDeleted"
  AND (cardinality(@states::text[]) = 0 OR "State" = ANY(@states::text[]))
  AND (@ref::text = '' OR "Ref" = @ref::text);

-- name: ListRuns :many
SELECT *
FROM "PipelineRuns"
WHERE "ProjectId" = @project_id AND "OrganizationId" = @organization_id AND NOT "IsDeleted"
  AND (cardinality(@states::text[]) = 0 OR "State" = ANY(@states::text[]))
  AND (@ref::text = '' OR "Ref" = @ref::text)
ORDER BY "Number" DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ClaimPendingRun :one
UPDATE "PipelineRuns"
SET "ClaimedUntil" = @claimed_until::timestamptz, "UpdatedAt" = now()
WHERE "Id" = (SELECT r."Id"
              FROM "PipelineRuns" r
              WHERE r."State" = 'pending' AND NOT r."IsDeleted" AND (r."ClaimedUntil" IS NULL OR r."ClaimedUntil" < @now::timestamptz)
                AND NOT EXISTS (SELECT 1
                                FROM "RunJobs" d
                                WHERE d."RunId" = r."Id" AND d."Kind" = 'discovery' AND d."State" IN ('waiting', 'active'))
              ORDER BY r."CreatedAt"
              LIMIT 1
              FOR UPDATE SKIP LOCKED)
RETURNING *;

-- name: ReleaseRunClaim :exec
UPDATE "PipelineRuns"
SET "ClaimedUntil" = NULL, "UpdatedAt" = now()
WHERE "Id" = @id AND "State" = 'pending';

-- name: SetRunDefinition :exec
UPDATE "PipelineRuns"
SET "DefinitionSource" = @definition_source, "DefinitionDigest" = @definition_digest, "ImagePins" = @image_pins,
    "Issues" = @issues, "Inputs" = @inputs, "ConcurrencyGroup" = sqlc.narg('concurrency_group')::text, "CancelInProgress" = @cancel_in_progress,
    "ClaimedUntil" = NULL, "UpdatedAt" = now()
WHERE "Id" = @id AND "State" = 'pending';

-- name: SetRunSource :exec
UPDATE "PipelineRuns"
SET "DefinitionSource" = @definition_source, "DefinitionDigest" = @definition_digest,
    "Sha" = COALESCE("Sha", sqlc.narg('sha')::text), "DefinitionSha" = COALESCE("DefinitionSha", sqlc.narg('sha')::text),
    "ClaimedUntil" = NULL, "UpdatedAt" = now()
WHERE "Id" = @id AND "State" = 'pending';

-- name: SetRunIssues :exec
UPDATE "PipelineRuns"
SET "Issues" = @issues, "UpdatedAt" = now()
WHERE "Id" = @id;

-- name: QueueRun :exec
UPDATE "PipelineRuns"
SET "State" = 'queued', "UpdatedAt" = now()
WHERE "Id" = @id AND "State" = 'pending';

-- name: StartRun :exec
UPDATE "PipelineRuns"
SET "State" = 'running', "StartedAt" = COALESCE("StartedAt", now()), "ClaimedUntil" = NULL, "UpdatedAt" = now()
WHERE "Id" = @id AND "State" IN ('pending', 'queued');

-- name: FinishRun :exec
UPDATE "PipelineRuns"
SET "State" = @state, "FailureReason" = @failure_reason, "FinishedAt" = now(), "ClaimedUntil" = NULL, "UpdatedAt" = now()
WHERE "Id" = @id AND "State" IN ('awaiting_approval', 'pending', 'queued', 'running');

-- name: ApproveRun :exec
UPDATE "PipelineRuns"
SET "State" = 'pending', "ApprovedAt" = now(), "ApprovedByUserId" = @user_id, "UpdatedAt" = now()
WHERE "Id" = @id AND "State" = 'awaiting_approval';

-- name: RequestRunCancel :exec
UPDATE "PipelineRuns"
SET "CancelRequestedAt" = COALESCE("CancelRequestedAt", now()),
    "CancelRequestedBy" = COALESCE("CancelRequestedBy", sqlc.narg('user_id')::uuid),
    "FailureReason" = CASE WHEN "CancelRequestedAt" IS NULL THEN @reason ELSE "FailureReason" END, "UpdatedAt" = now()
WHERE "Id" = @id;

-- name: ReopenRun :exec
UPDATE "PipelineRuns"
SET "State" = 'running', "FailureReason" = '', "FinishedAt" = NULL, "CancelRequestedAt" = NULL, "CancelRequestedBy" = NULL,
    "UpdatedAt" = now()
WHERE "Id" = @id AND "State" IN ('failed', 'canceled');

-- name: LockConcurrencyGroup :exec
SELECT pg_advisory_xact_lock(hashtextextended('dhow.run-group:' || (@project_id::uuid)::text || ':' || @concurrency_group::text, 0));

-- name: GroupBlocked :one
SELECT EXISTS (SELECT 1
               FROM "PipelineRuns"
               WHERE "ProjectId" = @project_id AND "ConcurrencyGroup" = @concurrency_group::text AND "Id" <> @id
                 AND NOT "IsDeleted"
                 AND ("State" = 'running' OR ("State" = 'queued' AND "Number" < @number)))::boolean;

-- name: OlderGroupRuns :many
SELECT "Id"
FROM "PipelineRuns"
WHERE "ProjectId" = @project_id AND "ConcurrencyGroup" = @concurrency_group::text AND "Id" <> @id AND "Number" < @number
  AND "Ref" = @ref AND "State" IN ('queued', 'running') AND NOT "IsDeleted"
ORDER BY "Number";

-- name: ListQueuedRuns :many
SELECT h."Id", h."ProjectId", h."ConcurrencyGroup"
FROM (SELECT DISTINCT ON (r."ProjectId", r."ConcurrencyGroup") r."Id", r."ProjectId", r."ConcurrencyGroup", r."Number"
      FROM "PipelineRuns" r
      WHERE r."State" = 'queued' AND NOT r."IsDeleted"
      ORDER BY r."ProjectId", r."ConcurrencyGroup", r."Number") h
WHERE NOT EXISTS (SELECT 1
                  FROM "PipelineRuns" o
                  WHERE o."ProjectId" = h."ProjectId" AND o."ConcurrencyGroup" = h."ConcurrencyGroup" AND o."State" = 'running'
                    AND NOT o."IsDeleted")
ORDER BY h."Number", h."Id"
LIMIT @batch;

-- name: TouchRun :exec
UPDATE "PipelineRuns"
SET "UpdatedAt" = now()
WHERE "Id" = @id;

-- name: InsertRunJob :one
INSERT INTO "RunJobs" ("OrganizationId", "RunId", "Kind", "Key", "JobName", "JobId", "Matrix", "Needs", "Position", "Digest")
VALUES (@organization_id, @run_id, @kind, @key, @job_name, @job_id, @matrix, @needs::text[], @position, @digest)
RETURNING *;

-- name: ListRunJobs :many
SELECT *
FROM "RunJobs"
WHERE "RunId" = @run_id AND NOT "IsDeleted"
ORDER BY "Position";

-- name: GetRunJob :one
SELECT *
FROM "RunJobs"
WHERE "Id" = @id AND "OrganizationId" = @organization_id;

-- name: ActivateRunJob :execrows
UPDATE "RunJobs"
SET "State" = 'active', "CurrentJobId" = @job_id, "Attempts" = "Attempts" + 1, "EnvironmentId" = @environment_id,
    "Gate" = NULL, "StartedAt" = now(), "UpdatedAt" = now()
WHERE "Id" = @id AND "State" = 'waiting';

-- name: FinishRunJob :execrows
UPDATE "RunJobs"
SET "State" = @state, "FailureReason" = @failure_reason, "Outputs" = @outputs, "Gate" = NULL,
    "FinishedAt" = now(), "UpdatedAt" = now()
WHERE "Id" = @id AND "State" IN ('waiting', 'active');

-- name: ResetRunJob :exec
UPDATE "RunJobs"
SET "State" = 'waiting', "FailureReason" = '', "FailureDetail" = '{}', "Outputs" = '{}', "StartedAt" = NULL, "FinishedAt" = NULL,
    "EnvironmentId" = NULL, "Gate" = NULL, "GateOpensAt" = NULL, "UpdatedAt" = now()
WHERE "Id" = @id AND "State" IN ('failed', 'canceled', 'skipped');

-- name: ListRunCancellations :many
SELECT rj."CurrentJobId"::uuid AS "JobId", r."OrganizationId", r."CancelRequestedBy"
FROM "PipelineRuns" r
JOIN "RunJobs" rj ON rj."RunId" = r."Id" AND rj."State" = 'active'
JOIN "Jobs" j ON j."Id" = rj."CurrentJobId"
WHERE r."CancelRequestedAt" IS NOT NULL AND r."State" = 'running' AND NOT r."IsDeleted"
  AND j."CancelRequestedAt" IS NULL AND j."State" IN ('queued', 'leased', 'running')
ORDER BY r."CancelRequestedAt"
LIMIT @batch;

-- name: ListDriftedRunJobs :many
SELECT rj."Id", rj."RunId", rj."OrganizationId", rj."CurrentJobId"::uuid AS "JobId", j."State" AS "JobState", j."FailureReason" AS "JobFailureReason"
FROM "RunJobs" rj
JOIN "Jobs" j ON j."Id" = rj."CurrentJobId"
WHERE rj."State" = 'active' AND NOT rj."IsDeleted" AND j."State" IN ('succeeded', 'failed', 'canceled', 'timed_out', 'lost')
  AND j."FinishedAt" < @finished_before::timestamptz
ORDER BY j."FinishedAt"
LIMIT @batch;

-- name: ListStalledRuns :many
SELECT r."Id"
FROM "PipelineRuns" r
WHERE r."State" = 'pending' AND NOT r."IsDeleted" AND r."CreatedAt" < @created_before::timestamptz AND r."DefinitionSource" IS NULL
  AND (r."ClaimedUntil" IS NULL OR r."ClaimedUntil" < now())
  AND NOT EXISTS (SELECT 1
                  FROM "RunJobs" d
                  WHERE d."RunId" = r."Id" AND d."Kind" = 'discovery' AND d."State" IN ('waiting', 'active'))
ORDER BY r."CreatedAt"
LIMIT @batch;

-- name: ListUnsettledRuns :many
SELECT r."Id"
FROM "PipelineRuns" r
WHERE r."State" = 'running' AND NOT r."IsDeleted" AND r."UpdatedAt" < @updated_before::timestamptz
  AND NOT EXISTS (SELECT 1
                  FROM "RunJobs" rj
                  WHERE rj."RunId" = r."Id" AND rj."State" = 'active')
  AND NOT EXISTS (SELECT 1
                  FROM "RunJobs" rj
                  WHERE rj."RunId" = r."Id" AND rj."State" = 'waiting' AND rj."Gate" IS NOT NULL AND NOT rj."IsDeleted")
ORDER BY r."UpdatedAt"
LIMIT @batch;

-- name: GetCheckoutRepository :one
SELECT "Id", "CloneUrl", "Transport", "HostKey", ("DeployKeySealed" IS NOT NULL)::boolean AS "HasDeployKey"
FROM "Repositories"
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND NOT "IsDeleted";

-- name: DiscoveryLog :many
SELECT c."Data"
FROM "LogChunks" c
WHERE c."JobId" = @job_id AND c."Stream" = 'stdout' AND c."StepIndex" = 0 AND NOT c."IsDeleted"
  AND c."AttemptId" = (SELECT a."Id"
                       FROM "JobAttempts" a
                       WHERE a."JobId" = @job_id
                       ORDER BY a."Number" DESC
                       LIMIT 1)
ORDER BY c."Seq";

-- name: CheckoutRepositoryOfJob :one
SELECT r."RepositoryId"::uuid AS "RepositoryId", r."ProjectId"
FROM "Jobs" j
JOIN "RunJobs" rj ON rj."Id" = j."OriginId" AND rj."OrganizationId" = j."OrganizationId"
    AND rj."CurrentJobId" = j."Id" AND rj."State" = 'active' AND NOT rj."IsDeleted"
JOIN "PipelineRuns" r ON r."Id" = rj."RunId" AND r."OrganizationId" = rj."OrganizationId"
    AND r."State" IN ('pending', 'running') AND NOT r."IsDeleted"
WHERE j."Id" = @job_id AND j."OrganizationId" = @organization_id AND j."OriginKind" = 'pipeline' AND NOT j."IsDeleted"
  AND r."RepositoryId" IS NOT NULL;

-- name: GetManualDefinition :one
SELECT r."DefaultBranch", d."DefinitionSource", d."DefinitionSha"
FROM "Projects" p
JOIN "Repositories" r ON r."ProjectId" = p."Id" AND r."OrganizationId" = p."OrganizationId" AND NOT r."IsDeleted"
LEFT JOIN LATERAL (
    SELECT pr."DefinitionSource", pr."DefinitionSha"
    FROM "PipelineRuns" pr
    WHERE pr."ProjectId" = p."Id" AND pr."OrganizationId" = p."OrganizationId" AND pr."RepositoryId" = r."Id"
      AND pr."Ref" = 'refs/heads/' || r."DefaultBranch" AND NOT pr."IsFork" AND pr."DefinitionPath" = p."PipelinePath"
      AND pr."DefinitionSource" IS NOT NULL AND NOT pr."IsDeleted"
    ORDER BY pr."Number" DESC
    LIMIT 1
) d ON true
WHERE p."Id" = @project_id AND p."OrganizationId" = @organization_id AND NOT p."IsDeleted";

-- name: ListOpenProjectRuns :many
SELECT "Id"
FROM "PipelineRuns"
WHERE "ProjectId" = @project_id AND "OrganizationId" = @organization_id AND NOT "IsDeleted"
  AND "State" IN ('awaiting_approval', 'pending', 'queued', 'running')
ORDER BY "Number"
FOR UPDATE;

-- name: CountRecentWebhookRuns :one
SELECT count(*)::bigint AS "Runs",
       COALESCE(ceil(EXTRACT(EPOCH FROM (min("CreatedAt") + make_interval(secs => @window_seconds::double precision) - now()))), 0)::bigint AS "RetryAfterSeconds"
FROM "PipelineRuns"
WHERE "ProjectId" = @project_id AND "DeliveryId" IS NOT NULL
  AND "CreatedAt" > now() - make_interval(secs => @window_seconds::double precision);

-- name: SecretScopeOfJob :one
SELECT r."ProjectId", r."Id" AS "RunId", rj."Id" AS "RunJobId", r."IsFork", (r."Trigger" = 'pull_request')::boolean AS "PullRequest",
       rj."EnvironmentId", r."Ref", r."BaseRef"
FROM "Jobs" j
JOIN "RunJobs" rj ON rj."Id" = j."OriginId" AND rj."OrganizationId" = j."OrganizationId"
    AND rj."CurrentJobId" = j."Id" AND rj."State" = 'active' AND NOT rj."IsDeleted"
JOIN "PipelineRuns" r ON r."Id" = rj."RunId" AND r."OrganizationId" = rj."OrganizationId"
    AND r."State" IN ('pending', 'running') AND NOT r."IsDeleted"
WHERE j."Id" = @job_id AND j."OrganizationId" = @organization_id AND j."OriginKind" = 'pipeline' AND NOT j."IsDeleted";

-- name: SetRunJobFailureDetail :exec
UPDATE "RunJobs" SET "FailureDetail" = @failure_detail::jsonb, "UpdatedAt" = now()
WHERE "Id" = @id AND "OrganizationId" = @organization_id;

-- name: SetRunJobGate :exec
UPDATE "RunJobs"
SET "Gate" = sqlc.narg('gate')::text, "GateOpensAt" = sqlc.narg('gate_opens_at')::timestamptz,
    "EnvironmentId" = sqlc.narg('environment_id')::uuid, "UpdatedAt" = now()
WHERE "Id" = @id AND "OrganizationId" = @organization_id AND "State" = 'waiting';

-- name: ListGatedRuns :many
SELECT r."Id"
FROM "PipelineRuns" r
WHERE r."State" = 'running' AND NOT r."IsDeleted" AND r."UpdatedAt" < @updated_before::timestamptz
  AND EXISTS (SELECT 1
              FROM "RunJobs" rj
              WHERE rj."RunId" = r."Id" AND rj."State" = 'waiting' AND rj."Gate" IS NOT NULL AND NOT rj."IsDeleted")
ORDER BY r."UpdatedAt"
LIMIT @batch;

-- name: EnvironmentQueueAhead :one
SELECT EXISTS (SELECT 1
               FROM "RunJobs" rj
               JOIN "PipelineRuns" r ON r."Id" = rj."RunId" AND r."OrganizationId" = rj."OrganizationId"
               WHERE rj."EnvironmentId" = @environment_id AND rj."OrganizationId" = @organization_id
                 AND rj."Gate" = 'environment_busy' AND rj."State" = 'waiting' AND NOT rj."IsDeleted"
                 AND r."State" = 'running' AND NOT r."IsDeleted" AND r."Id" <> @run_id AND r."Number" < @number)::boolean;

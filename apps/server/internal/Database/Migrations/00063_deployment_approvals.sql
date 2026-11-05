-- dhow:phase expand
-- +goose Up
CREATE TABLE "DeploymentApprovals" (
    "Id"              uuid PRIMARY KEY DEFAULT uuidv7(),
    "CreatedAt"       timestamptz NOT NULL DEFAULT now(),
    "UpdatedAt"       timestamptz NOT NULL DEFAULT now(),
    "IsDeleted"       boolean NOT NULL DEFAULT false,
    "OrganizationId"  uuid NOT NULL,
    "EnvironmentId"   uuid NOT NULL,
    "RunId"           uuid NOT NULL,
    "RunNumber"       bigint NOT NULL CHECK ("RunNumber" >= 1),
    "Ref"             TEXT NOT NULL CHECK (char_length("Ref") BETWEEN 1 AND 255),
    "Round"           integer NOT NULL CHECK ("Round" >= 1),
    "RuleVersion"     bigint NOT NULL CHECK ("RuleVersion" >= 1),
    "State"           TEXT NOT NULL DEFAULT 'pending'
                      CHECK ("State" IN ('pending', 'approved', 'rejected', 'expired', 'superseded', 'canceled', 'bypassed')),
    "ExpiresAt"       timestamptz NOT NULL,
    "DecidedByUserId" uuid REFERENCES "Users" ("Id"),
    "DecidedAt"       timestamptz,
    "FactorKind"      TEXT CHECK ("FactorKind" IN ('totp', 'recovery_code', 'passkey')),
    "Comment"         TEXT CHECK (octet_length("Comment") <= 1024),
    "BypassReason"    TEXT CHECK (octet_length("BypassReason") BETWEEN 1 AND 1024),
    "SupersededByRunId" uuid,
    FOREIGN KEY ("EnvironmentId", "OrganizationId") REFERENCES "Environments" ("Id", "OrganizationId"),
    CHECK (("State" = 'pending') = ("DecidedAt" IS NULL)),
    CHECK (("State" IN ('approved', 'rejected', 'bypassed')) = ("DecidedByUserId" IS NOT NULL)),
    CHECK (("DecidedByUserId" IS NULL) = ("FactorKind" IS NULL)),
    CHECK (("State" = 'bypassed') = ("BypassReason" IS NOT NULL)),
    CHECK ("Comment" IS NULL OR "State" IN ('approved', 'rejected')),
    CHECK ("SupersededByRunId" IS NULL OR "State" = 'superseded')
);

CREATE UNIQUE INDEX "UX_DeploymentApprovals_Live" ON "DeploymentApprovals" ("RunId", "EnvironmentId", "Round")
    WHERE "State" IN ('pending', 'approved', 'bypassed') AND NOT "IsDeleted";
CREATE INDEX "IX_DeploymentApprovals_Pending" ON "DeploymentApprovals" ("OrganizationId", "CreatedAt")
    WHERE "State" = 'pending' AND NOT "IsDeleted";
CREATE INDEX "IX_DeploymentApprovals_Ref" ON "DeploymentApprovals" ("EnvironmentId", "Ref")
    WHERE "State" = 'pending' AND NOT "IsDeleted";
CREATE INDEX "IX_DeploymentApprovals_Run" ON "DeploymentApprovals" ("RunId", "CreatedAt") WHERE NOT "IsDeleted";
CREATE UNIQUE INDEX "UX_DeploymentApprovals_Id_Organization" ON "DeploymentApprovals" ("Id", "OrganizationId");

-- +goose StatementBegin
CREATE FUNCTION "GuardDeploymentApproval"() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW."OrganizationId" <> OLD."OrganizationId" OR NEW."EnvironmentId" <> OLD."EnvironmentId" OR NEW."RunId" <> OLD."RunId"
        OR NEW."Round" <> OLD."Round" OR NEW."RuleVersion" <> OLD."RuleVersion" OR NEW."Ref" <> OLD."Ref" THEN
        RAISE EXCEPTION 'what an approval is for never changes' USING ERRCODE = 'check_violation';
    END IF;
    IF NEW."State" = OLD."State" THEN
        IF OLD."State" <> 'pending' AND (NEW."DecidedByUserId" IS DISTINCT FROM OLD."DecidedByUserId"
            OR NEW."DecidedAt" IS DISTINCT FROM OLD."DecidedAt" OR NEW."Comment" IS DISTINCT FROM OLD."Comment"
            OR NEW."BypassReason" IS DISTINCT FROM OLD."BypassReason" OR NEW."FactorKind" IS DISTINCT FROM OLD."FactorKind"
            OR NEW."SupersededByRunId" IS DISTINCT FROM OLD."SupersededByRunId") THEN
            RAISE EXCEPTION 'a decided approval keeps its decision' USING ERRCODE = 'check_violation';
        END IF;
        RETURN NEW;
    END IF;
    IF OLD."State" = 'pending' THEN
        RETURN NEW;
    END IF;
    IF OLD."State" IN ('approved', 'bypassed') AND NEW."State" = 'superseded' AND NEW."DecidedByUserId" IS NOT DISTINCT FROM OLD."DecidedByUserId" THEN
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'an approval moves from pending only, not from %', OLD."State" USING ERRCODE = 'check_violation';
END
$$;
-- +goose StatementEnd

CREATE TRIGGER "GuardDeploymentApproval" BEFORE UPDATE ON "DeploymentApprovals"
    FOR EACH ROW EXECUTE FUNCTION "GuardDeploymentApproval"();

ALTER TABLE "Deployments" ADD COLUMN "ApprovalId" uuid;
ALTER TABLE "PipelineRuns" ADD COLUMN "Round" integer NOT NULL DEFAULT 1;
ALTER TABLE "PipelineRuns" ADD CONSTRAINT "CK_PipelineRuns_Round" CHECK ("Round" >= 1) NOT VALID;
ALTER TABLE "UserIdentities" ADD COLUMN "Login" TEXT;
ALTER TABLE "UserIdentities" ADD CONSTRAINT "CK_UserIdentities_Login" CHECK (char_length("Login") BETWEEN 1 AND 255) NOT VALID;

-- +goose Down
ALTER TABLE "UserIdentities" DROP CONSTRAINT "CK_UserIdentities_Login";
ALTER TABLE "UserIdentities" DROP COLUMN "Login";
ALTER TABLE "PipelineRuns" DROP CONSTRAINT "CK_PipelineRuns_Round";
ALTER TABLE "PipelineRuns" DROP COLUMN "Round";
ALTER TABLE "Deployments" DROP COLUMN "ApprovalId";
DROP TABLE "DeploymentApprovals";
DROP FUNCTION "GuardDeploymentApproval"();

package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	RunAwaitingApproval = "awaiting_approval"
	RunPending          = "pending"
	RunQueued           = "queued"
	RunRunning          = "running"
	RunSucceeded        = "succeeded"
	RunFailed           = "failed"
	RunCanceled         = "canceled"
	RunSkipped          = "skipped"
)

const (
	JobWaiting   = "waiting"
	JobActive    = "active"
	JobSucceeded = "succeeded"
	JobFailed    = "failed"
	JobCanceled  = "canceled"
	JobSkipped   = "skipped"
)

const (
	KindJob       = "job"
	KindDiscovery = "discovery"
)

const (
	ReasonPipelineInvalid         = "PIPELINE_INVALID"
	ReasonCheckoutUnavailable     = "CHECKOUT_UNAVAILABLE"
	ReasonProviderUnavailable     = "PROVIDER_UNAVAILABLE"
	ReasonDefinitionNotFound      = "DEFINITION_NOT_FOUND"
	ReasonDefinitionTimeout       = "DEFINITION_TIMEOUT"
	ReasonDefinitionChanged       = "DEFINITION_CHANGED"
	ReasonDiscoveryFailed         = "DISCOVERY_FAILED"
	ReasonDefinitionUnverified    = "DEFINITION_UNVERIFIED"
	ReasonTriggerNotEnabled       = "TRIGGER_NOT_ENABLED"
	ReasonInputsInvalid           = "INPUTS_INVALID"
	ReasonImagePinFailed          = "IMAGE_PIN_FAILED"
	ReasonJobsFailed              = "JOBS_FAILED"
	ReasonCanceled                = "CANCELED"
	ReasonProjectArchived         = "PROJECT_ARCHIVED"
	ReasonNeedsFailed             = "NEEDS_FAILED"
	ReasonConditionFalse          = "CONDITION_FALSE"
	ReasonExpressionFailed        = "EXPRESSION_FAILED"
	ReasonEnvironments            = "ENVIRONMENTS_UNAVAILABLE"
	ReasonForkEnvironment         = "FORK_PR_NO_ENVIRONMENT"
	ReasonEnvironmentNotFound     = "ENVIRONMENT_NOT_FOUND"
	ReasonEnvironmentRef          = "ENVIRONMENT_REF_NOT_ALLOWED"
	ReasonEnvironmentPinnedImages = "ENVIRONMENT_REQUIRES_PINNED_IMAGES"
	ReasonEnvironmentPlugin       = "ENVIRONMENT_PLUGIN_NOT_ALLOWED"
	ReasonEnvironmentIsolation    = "ENVIRONMENT_ISOLATION_REQUIRED"
	ReasonEnvironmentRuleChanged  = "ENVIRONMENT_RULE_CHANGED"
	ReasonDeploymentRejected      = "DEPLOYMENT_REJECTED"
	ReasonApprovalExpired         = "APPROVAL_EXPIRED"
	ReasonSuperseded              = "SUPERSEDED"
	ReasonForkSecrets             = "FORK_PR_NO_SECRETS" //nolint:gosec // A failure reason, not a credential.
	ReasonSecretNotFound          = "SECRET_NOT_FOUND"   //nolint:gosec // A failure reason, not a credential.
	ReasonSecretNotForPRs         = "SECRET_NOT_AVAILABLE_TO_PULL_REQUESTS"
	ReasonSecretsTooLarge         = "SECRETS_TOO_LARGE" //nolint:gosec // A failure reason, not a credential.
	ReasonSecretsUnavailable      = "SECRETS_UNAVAILABLE"
	ReasonJobSpecInvalid          = "JOB_SPEC_INVALID"
	ReasonCreatorNotMember        = "CREATOR_NOT_MEMBER"
	ReasonEnqueueRefused          = "ENQUEUE_REFUSED"
	ReasonExpansionFailed         = "RUN_EXPANSION_FAILED"
	ReasonStateInvalid            = "RUN_STATE_INVALID"
	ReasonOutputsTruncated        = "OUTPUTS_TRUNCATED"
	ReasonRepositoryUnavailable   = "REPOSITORY_UNAVAILABLE"
	ReasonRepositoryUnlinked      = "REPOSITORY_UNLINKED"
	ReasonShaNotOnRef             = "SHA_NOT_ON_REF"
	ReasonArtifactUntrusted       = "ARTIFACT_UNTRUSTED"
	ReasonArtifactsUnavailable    = "ARTIFACTS_UNAVAILABLE"
)

const (
	ReasonTargetNotFound         = "TARGET_NOT_FOUND"
	ReasonTargetNotLiteral       = "TARGET_NOT_LITERAL"
	ReasonTargetNeedsEnvironment = "TARGET_REQUIRES_ENVIRONMENT"
	ReasonTargetHostKey          = "TARGET_HOST_KEY_UNCONFIRMED"
	ReasonTargetKeyNotFound      = "TARGET_KEY_NOT_FOUND"
	ReasonTargetPluginNotAllowed = "TARGET_PLUGIN_NOT_ALLOWED"
	ReasonTargetArtifact         = "TARGET_ARTIFACT_REQUIRED"
	ReasonTargetEgress           = "TARGET_JOB_EGRESS_NOT_ALLOWED"
	ReasonTargetChanged          = "TARGET_CHANGED"
	ReasonHealthURLInvalid       = "HEALTH_URL_INVALID"
	ReasonHealthURLNotAllowed    = "HEALTH_URL_NOT_ALLOWED"
	ReasonTargetPluginNotPinned  = "TARGET_PLUGIN_NOT_PINNED"
	ReasonTargetKeyReferenced    = "TARGET_KEY_NOT_REFERENCEABLE"
	ReasonHealthBudget           = "HEALTH_BUDGET_EXCEEDED"
	ReasonRollingBudget          = "ROLLING_BUDGET_EXCEEDED"
	ReasonRollbackReleaseGone    = "ROLLBACK_RELEASE_UNAVAILABLE"
	ReasonRollbackNotAllowed     = "ROLLBACK_MODE_NOT_ALLOWED"
)

const (
	ForkPolicyApproval  = "approval"
	ForkPolicyAutomatic = "automatic"
)

func RunStates() []string {
	return []string{RunAwaitingApproval, RunPending, RunQueued, RunRunning, RunSucceeded, RunFailed, RunCanceled, RunSkipped}
}

func RunFinished(state string) bool {
	switch state {
	case RunSucceeded, RunFailed, RunCanceled, RunSkipped:
		return true
	default:
		return false
	}
}

func JobFinished(state string) bool {
	switch state {
	case JobSucceeded, JobFailed, JobCanceled, JobSkipped:
		return true
	default:
		return false
	}
}

func Satisfied(state, reason string) bool {
	return state == JobSucceeded || (state == JobSkipped && reason != ReasonNeedsFailed)
}

type Issue struct {
	Code     string
	Severity string
	Path     string
	Line     int
	Column   int
	Message  string
}

type ImagePin struct {
	Image  string
	Pinned string
}

type Run struct {
	ID                uuid.UUID
	ProjectID         uuid.UUID
	Number            int64
	Trigger           string
	Ref               string
	Sha               string
	BaseRef           string
	BaseSha           string
	IsFork            bool
	PrNumber          int
	Actor             string
	State             string
	FailureReason     string
	DefinitionPath    string
	DefinitionSha     string
	DefinitionDigest  string
	ConcurrencyGroup  string
	TriggeredByUserID *uuid.UUID
	ApprovedByUserID  *uuid.UUID
	ApprovedAt        *time.Time
	CancelRequestedAt *time.Time
	StartedAt         *time.Time
	FinishedAt        *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type RunJob struct {
	ID            uuid.UUID
	Key           string
	Kind          string
	JobID         string
	Name          string
	Matrix        map[string]string
	Needs         []string
	State         string
	FailureReason string
	FailureNames  []string
	Environment   string
	EnvironmentID *uuid.UUID
	Gate          string
	GateOpensAt   *time.Time
	CurrentJobID  *uuid.UUID
	Attempts      int
	StartedAt     *time.Time
	FinishedAt    *time.Time
}

type Detail struct {
	Run       Run
	Jobs      []RunJob
	Issues    []Issue
	ImagePins []ImagePin
}

type Page struct {
	Items []Run
	Total int64
}

type TriggerInput struct {
	Ref    string
	Sha    string
	Inputs map[string]any
}

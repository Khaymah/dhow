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
	ReasonPipelineInvalid     = "PIPELINE_INVALID"
	ReasonCheckoutUnavailable = "CHECKOUT_UNAVAILABLE"
	ReasonProviderUnavailable = "PROVIDER_UNAVAILABLE"
	ReasonDefinitionNotFound  = "DEFINITION_NOT_FOUND"
	ReasonDefinitionTimeout   = "DEFINITION_TIMEOUT"
	ReasonDefinitionChanged   = "DEFINITION_CHANGED"
	ReasonDiscoveryFailed     = "DISCOVERY_FAILED"
	ReasonTriggerNotEnabled   = "TRIGGER_NOT_ENABLED"
	ReasonInputsInvalid       = "INPUTS_INVALID"
	ReasonImagePinFailed      = "IMAGE_PIN_FAILED"
	ReasonJobsFailed          = "JOBS_FAILED"
	ReasonCanceled            = "CANCELED"
	ReasonProjectArchived     = "PROJECT_ARCHIVED"
	ReasonNeedsFailed         = "NEEDS_FAILED"
	ReasonConditionFalse      = "CONDITION_FALSE"
	ReasonExpressionFailed    = "EXPRESSION_FAILED"
	ReasonEnvironments        = "ENVIRONMENTS_UNAVAILABLE"
	ReasonForkEnvironment     = "FORK_PR_NO_ENVIRONMENT"
	ReasonForkSecrets         = "FORK_PR_NO_SECRETS" //nolint:gosec // A failure reason, not a credential.
	ReasonSecretsUnavailable  = "SECRETS_UNAVAILABLE"
	ReasonJobSpecInvalid      = "JOB_SPEC_INVALID"
	ReasonCreatorNotMember    = "CREATOR_NOT_MEMBER"
	ReasonEnqueueRefused      = "ENQUEUE_REFUSED"
	ReasonExpansionFailed     = "RUN_EXPANSION_FAILED"
	ReasonStateInvalid        = "RUN_STATE_INVALID"
	ReasonOutputsTruncated    = "OUTPUTS_TRUNCATED"
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
	JobID         string
	Name          string
	Matrix        map[string]string
	Needs         []string
	State         string
	FailureReason string
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

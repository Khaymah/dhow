package models

import "slices"

const (
	JobQueued    = "queued"
	JobLeased    = "leased"
	JobRunning   = "running"
	JobSucceeded = "succeeded"
	JobFailed    = "failed"
	JobCanceled  = "canceled"
	JobTimedOut  = "timed_out"
	JobLost      = "lost"
)

const (
	AttemptLeased    = "leased"
	AttemptRunning   = "running"
	AttemptSucceeded = "succeeded"
	AttemptFailed    = "failed"
	AttemptCanceled  = "canceled"
	AttemptTimedOut  = "timed_out"
	AttemptLost      = "lost"
)

const (
	LeaseActive   = "active"
	LeaseReleased = "released"
	LeaseExpired  = "expired"
	LeaseRevoked  = "revoked"
)

const (
	StepRunning     = "running"
	StepSucceeded   = "succeeded"
	StepFailed      = "failed"
	StepInterrupted = "interrupted"
)

const (
	ReasonCanceled          = "CANCELED"
	ReasonQueueTimeout      = "QUEUE_TIMEOUT"
	ReasonLostAfterMutation = "LOST_AFTER_MUTATION"
	ReasonRetriesExhausted  = "RETRIES_EXHAUSTED"
	ReasonLeaseLost         = "LEASE_LOST"
	ReasonRunnerRevoked     = "RUNNER_REVOKED"
	ReasonTimedOut          = "TIMED_OUT"
	ReasonStartTimeout      = "START_TIMEOUT"
	ReasonSecretsMissing    = "SECRETS_UNAVAILABLE"
	ReasonEnvironmentRule   = "ENVIRONMENT_RULE_CHANGED"
	ReasonTargetChanged     = "TARGET_CHANGED"
	ReasonTargetKeyMissing  = "TARGET_KEY_NOT_FOUND"
	ReasonTargetKeyUsed     = "TARGET_KEY_NOT_REFERENCEABLE"
	ReasonNoMatchingRunner  = "NO_MATCHING_RUNNER"
)

func JobStates() []string {
	return []string{JobQueued, JobLeased, JobRunning, JobSucceeded, JobFailed, JobCanceled, JobTimedOut, JobLost}
}

func AttemptStates() []string {
	return []string{AttemptLeased, AttemptRunning, AttemptSucceeded, AttemptFailed, AttemptCanceled, AttemptTimedOut, AttemptLost}
}

func LeaseStates() []string {
	return []string{LeaseActive, LeaseReleased, LeaseExpired, LeaseRevoked}
}

func StepStates() []string {
	return []string{StepRunning, StepSucceeded, StepFailed, StepInterrupted}
}

func JobTransitions() map[string][]string {
	return map[string][]string{
		JobQueued:  {JobLeased, JobCanceled, JobFailed},
		JobLeased:  {JobRunning, JobQueued, JobCanceled, JobFailed, JobLost},
		JobRunning: {JobQueued, JobSucceeded, JobFailed, JobCanceled, JobTimedOut, JobLost},
	}
}

func AttemptTransitions() map[string][]string {
	return map[string][]string{
		AttemptLeased:  {AttemptRunning, AttemptFailed, AttemptCanceled, AttemptLost},
		AttemptRunning: {AttemptSucceeded, AttemptFailed, AttemptCanceled, AttemptTimedOut, AttemptLost},
	}
}

func LeaseTransitions() map[string][]string {
	return map[string][]string{LeaseActive: {LeaseReleased, LeaseExpired, LeaseRevoked}}
}

func StepTransitions() map[string][]string {
	return map[string][]string{StepRunning: {StepSucceeded, StepFailed, StepInterrupted}}
}

func allowed(table map[string][]string, from, to string) bool {
	return from == to || slices.Contains(table[from], to)
}

func JobCanMove(from, to string) bool { return allowed(JobTransitions(), from, to) }

func AttemptCanMove(from, to string) bool { return allowed(AttemptTransitions(), from, to) }

func LeaseCanMove(from, to string) bool { return allowed(LeaseTransitions(), from, to) }

func StepCanMove(from, to string) bool { return allowed(StepTransitions(), from, to) }

func JobFinished(state string) bool {
	return state != JobQueued && state != JobLeased && state != JobRunning
}

func AttemptFinished(state string) bool {
	return state != AttemptLeased && state != AttemptRunning
}

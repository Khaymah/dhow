package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	RefBranch = "branch"
	RefTag    = "tag"

	IsolationContainer = "container"

	DefaultBranch               = "main"
	DefaultApprovalTimeoutHours = 72
	RequiredApprovals           = 1

	MaxAllowedRefs     = 50
	MaxAllowedPlugins  = 50
	MaxPluginPrefix    = 256
	MaxReviewers       = 20
	MaxDescription     = 500
	MaxPauseReason     = 500
	MaxWaitMinutes     = 43200
	MinApprovalTimeout = 1
	MaxApprovalTimeout = 720
)

type Ref struct {
	Kind    string `json:"Kind"`
	Pattern string `json:"Pattern"`
}

type Reviewers struct {
	UserIDs []uuid.UUID
	TeamIDs []uuid.UUID
}

type Rules struct {
	AllowedRefs          []Ref
	AllowPullRequests    bool
	Reviewers            Reviewers
	RequiredApprovals    int
	AllowSelfApproval    bool
	WaitMinutes          int
	ApprovalTimeoutHours int
	RequirePinnedImages  bool
	AllowedPlugins       []string
	RequiredIsolation    string
	AllowAdminBypass     bool
	SupersedePending     bool

	AllowExtraDeployEgress bool
}

type RulesInput struct {
	AllowedRefs          *[]Ref
	AllowPullRequests    *bool
	Reviewers            *Reviewers
	RequiredApprovals    *int
	AllowSelfApproval    *bool
	WaitMinutes          *int
	ApprovalTimeoutHours *int
	RequirePinnedImages  *bool
	AllowedPlugins       *[]string
	RequiredIsolation    *string
	AllowAdminBypass     *bool
	SupersedePending     *bool

	AllowExtraDeployEgress *bool
}

type Environment struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	ProjectID      uuid.UUID
	Name           string
	Description    string
	Version        int64
	RuleVersion    int64
	Rules          Rules
	PausedAt       *time.Time
	PausedByUserID *uuid.UUID
	PausedReason   string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type CreateInput struct {
	Name        string
	Description *string
	Rules       *RulesInput
}

type UpdateInput struct {
	Version     int64
	Description *string
	Rules       *RulesInput
}

type Page struct {
	Items []Environment
	Total int64
}

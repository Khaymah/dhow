package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	StateActive   = "active"
	StateArchived = "archived"

	ForkPolicyApproval  = "approval"
	ForkPolicyAutomatic = "automatic"
	ForkCacheNone       = "none"
	ForkCacheDefault    = "default_branch"

	DefaultPipelinePath = ".dhow.yml"
)

type Project struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID
	Slug           string
	Name           string
	PipelinePath   string
	ForkPolicy     string
	ForkCache      string
	State          string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	ArchivedAt     *time.Time
}

type CreateInput struct {
	Name         string
	Slug         string
	PipelinePath *string
	ForkPolicy   *string
}

type UpdateInput struct {
	Name         *string
	PipelinePath *string
	ForkPolicy   *string
	ForkCache    *string
}

type Page struct {
	Items []Project
	Total int64
}

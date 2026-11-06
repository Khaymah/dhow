package models

import (
	"time"

	"github.com/google/uuid"

	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
)

const (
	ScopeOrganization = "organization"
	ScopeProject      = "project"
	ScopeEnvironment  = "environment"

	VisibilityAll      = "all_projects"
	VisibilitySelected = "selected_projects"
)

type Ref struct {
	OrganizationID  uuid.UUID
	ProjectID       *uuid.UUID
	EnvironmentName string
}

type Target struct {
	Scope          string
	OrganizationID uuid.UUID
	ProjectID      *uuid.UUID
	EnvironmentID  *uuid.UUID
}

type Secret struct {
	Name                    string
	Scope                   string
	Description             string
	Version                 int32
	SizeBytes               int32
	AvailableToPullRequests bool
	Visibility              string
	ProjectIDs              []uuid.UUID
	Overrides               []string
	CreatedAt               time.Time
	UpdatedAt               time.Time
	UpdatedByUserID         uuid.UUID
	LastUsedAt              *time.Time
}

type PutSecretInput struct {
	Value                   string
	Description             *string
	AvailableToPullRequests *bool
	Visibility              *string
	ProjectIDs              *[]uuid.UUID
	Factor                  *auth.Factor
}

type Variable struct {
	Name            string
	Scope           string
	Value           string
	Overrides       []string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	UpdatedByUserID uuid.UUID
}

type RegistryCredential struct {
	ID              uuid.UUID
	Scope           string
	Host            string
	Username        string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	UpdatedByUserID uuid.UUID
}

type RegistryInput struct {
	Host     string
	Username string
	Password string
}

type Page[T any] struct {
	Items []T
	Total int64
}

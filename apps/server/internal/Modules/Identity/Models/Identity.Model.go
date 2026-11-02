package models

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID     uuid.UUID
	Email  string
	Name   string
	Locale string
}

type Organization struct {
	ID                       uuid.UUID
	Name                     string
	Slug                     string
	CreatedAt                time.Time
	RequireSecondFactor      string
	RequireSecondFactorSince *time.Time
	LogRetentionDays         *int32
}

type Member struct {
	UserID          uuid.UUID
	Name            string
	Email           string
	Role            string
	EffectiveRole   string
	JoinedAt        time.Time
	LastActiveAt    *time.Time
	Mfa             string
	MfaResetPending bool
	Teams           []MemberTeam
}

type MemberTeam struct {
	ID   uuid.UUID
	Name string
}

type SetupInput struct {
	Token            string
	OwnerName        string
	Email            string
	Password         string
	OrganizationName string
	OrganizationSlug string
	Locale           string
}

type SetupResult struct {
	User         User
	Organization Organization
}

type MemberChange struct {
	UserID  uuid.UUID
	Role    string
	Revoked int64
}

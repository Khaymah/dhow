package models

import (
	"time"

	"github.com/google/uuid"

	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const (
	StateActive   = "active"
	StateDisabled = "disabled"
	StateRevoked  = "revoked"

	StatusOnline   = "online"
	StatusOffline  = "offline"
	StatusDisabled = "disabled"
	StatusRevoked  = "revoked"

	DisabledManual         = "manual"
	DisabledCloneSuspected = "clone_suspected"

	TokenActive  = "active"
	TokenExpired = "expired"
	TokenUsedUp  = "used_up"
	TokenRevoked = "revoked"

	IsolationContainer = "container"
	IsolationGvisor    = "gvisor"
)

type Runner struct {
	ID               uuid.UUID
	OrganizationID   uuid.UUID
	Name             string
	Labels           []string
	CapabilityLabels []string
	Os               string
	Arch             string
	Version          string
	IsolationLevels  []string
	Capacity         int
	State            string
	Status           string
	DisabledReason   string
	KeyVersion       int
	KeysRotatedAt    time.Time
	LastSeenAt       *time.Time
	CreatedAt        time.Time
	RevokedAt        *time.Time
}

type RegistrationToken struct {
	ID              uuid.UUID
	Description     string
	Labels          []string
	MaxUses         int
	Uses            int
	ExpiresAt       time.Time
	Status          string
	CreatedAt       time.Time
	CreatedByUserID uuid.UUID
	LastUsedAt      *time.Time
	RevokedAt       *time.Time
}

type CreatedToken struct {
	Token           RegistrationToken
	Secret          string
	RegisterCommand string
}

type TokenInput struct {
	Description *string
	Labels      *[]string
	MaxUses     *int
	ExpiresIn   *int
}

type UpdateInput struct {
	Name     *string
	Labels   *[]string
	Disabled *bool
}

type RegisterInput struct {
	Token               string
	Name                string
	Os                  string
	Arch                string
	Version             string
	IsolationLevels     []string
	Capacity            int
	SigningPublicKey    []byte
	EncryptionPublicKey []byte
	Proof               []byte
	Features            []runnerv1.RunnerFeature
}

type RegisterResult struct {
	RunnerID         uuid.UUID
	OrganizationID   uuid.UUID
	Labels           []string
	ServerSigningKey []byte
	ServerKeyVersion int32
}

type AuthenticateInput struct {
	RunnerID       string
	ChallengeBytes []byte
	ChallengeMAC   []byte
	Signature      []byte
	Features       []runnerv1.RunnerFeature
}

type RotateInput struct {
	NewSigningPublicKey    []byte
	NewEncryptionPublicKey []byte
	OldKeySignature        []byte
	NewKeySignature        []byte
}

package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	RefKindBranch = "branch"
	RefKindTag    = "tag"

	StatusActive  = "active"
	StatusExpired = "expired"
	StatusRevoked = "revoked"

	ReasonTriggerDisabled   = "TRIGGER_DISABLED"
	ReasonNoArtifactTrigger = "NO_ARTIFACT_TRIGGER"
)

type RefPattern struct {
	Kind    string
	Pattern string
}

type Token struct {
	ID              uuid.UUID
	ProjectID       uuid.UUID
	Name            string
	Prefix          string
	RefPatterns     []RefPattern
	Status          string
	CreatedAt       time.Time
	ExpiresAt       time.Time
	LastUsedAt      *time.Time
	RevokedAt       *time.Time
	CreatedByUserID uuid.UUID
	Reaches         []string

	CreatorAllowed      bool
	UnacknowledgedReach []string
}

type TokenPage struct {
	Items []Token
	Total int64
}

type CreateTokenInput struct {
	Name          string
	RefPatterns   []RefPattern
	ExpiresInDays int

	AcknowledgeProtectedReach bool
}

type CreatedToken struct {
	Token Token
	Value string
}

type Run struct {
	ID     uuid.UUID
	Number int64
	State  string
}

type Artifact struct {
	ID                 uuid.UUID
	Name               string
	Digest             string
	SizeBytes          int64
	Ref                string
	RefKind            string
	Sha                string
	Provenance         map[string]string
	ProvenanceVerified bool
	TokenName          string
	Run                *Run
	CreatedAt          time.Time
	ExpiresAt          time.Time
	PinnedUntil        *time.Time
}

type ArtifactPage struct {
	Items []Artifact
	Total int64
}

type Received struct {
	ArtifactID uuid.UUID
	Name       string
	Digest     string
	SizeBytes  int64
	Created    bool
	Triggered  bool
	Reason     string
	Run        *Run
	RunURL     string
}

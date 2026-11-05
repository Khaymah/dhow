package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	TargetKindSSH     = "ssh"
	TargetKindCompose = "compose"

	NotReadyHostKey = "host_key_unconfirmed"
	NotReadyKey     = "key_missing"

	DefaultSSHPort       = 22
	DefaultKeepReleases  = 5
	MinKeepReleases      = 2
	MaxKeepReleases      = 50
	MaxTargetHosts       = 10
	MaxAllowedImages     = 10
	MaxSharedNames       = 20
	MaxHookLength        = 1024
	MaxBatchPauseSeconds = 600
	MaxSettingsBytes     = 4096
	DefaultComposeFile   = "compose.yaml"
	DefaultBatchSize     = 1
	RootUsername         = "root"
	HostKeySourceFetched = "fetched"
	HostKeySourcePasted  = "pasted"
)

type Rolling struct {
	BatchSize         int `json:"BatchSize"`
	BatchPauseSeconds int `json:"BatchPauseSeconds"`
}

type SSHSettings struct {
	ReloadCommand     string   `json:"ReloadCommand"`
	PreSwitchCommand  string   `json:"PreSwitchCommand"`
	PostSwitchCommand string   `json:"PostSwitchCommand"`
	Shared            []string `json:"Shared"`
}

type ComposeSettings struct {
	ProjectName        string `json:"ProjectName"`
	ComposeFile        string `json:"ComposeFile"`
	AllowUnsafeCompose bool   `json:"AllowUnsafeCompose"`
}

type TargetSettings struct {
	Rolling   *Rolling         `json:"Rolling,omitempty"`
	AllowRoot bool             `json:"AllowRoot"`
	SSH       *SSHSettings     `json:"Ssh,omitempty"`
	Compose   *ComposeSettings `json:"Compose,omitempty"`
}

type TargetHost struct {
	ID                        uuid.UUID
	Position                  int
	Host                      string
	Port                      int
	KeyPinned                 bool
	HostKeyFingerprint        string
	HostKeyConfirmedAt        *time.Time
	HostKeyConfirmedByUserID  *uuid.UUID
	PendingHostKeyFingerprint string
}

type Target struct {
	ID            uuid.UUID
	EnvironmentID uuid.UUID
	Name          string
	Kind          string
	Username      string
	KeySecretName string
	PublicKey     string
	DeployPath    string
	KeepReleases  int
	Settings      TargetSettings
	AllowedImages []string
	Hosts         []TargetHost
	Ready         bool
	NotReady      []string
	Version       int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type HostInput struct {
	Host string
	Port *int
}

type CreateTargetInput struct {
	Name          string
	Kind          string
	Username      string
	KeySecretName *string
	DeployPath    string
	KeepReleases  *int
	Settings      *TargetSettings
	AllowedImages *[]string
	Hosts         []HostInput
}

type UpdateTargetInput struct {
	Version       int64
	Username      *string
	KeySecretName *string
	DeployPath    *string
	KeepReleases  *int
	Settings      *TargetSettings
	AllowedImages *[]string
}

type UpdateHostInput struct {
	Version int64
	Host    *string
	Port    *int
}

type TargetPage struct {
	Items []Target
	Total int64
}

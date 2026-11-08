package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	KindEmail    = "email"
	KindWebhook  = "webhook"
	KindSlack    = "slack"
	KindTelegram = "telegram"
	KindDiscord  = "discord"

	ReasonCreatorRemoved = "creator-removed"
	ReasonFailures       = "failures"

	StatePending = "pending"
	StateSending = "sending"
	StateSent    = "sent"
	StateDead    = "dead"
)

func Kinds() []string {
	return []string{KindEmail, KindWebhook, KindSlack, KindTelegram, KindDiscord}
}

func Chat(kind string) bool {
	return kind == KindSlack || kind == KindTelegram || kind == KindDiscord
}

type Scope struct {
	OrganizationID uuid.UUID
	ProjectID      *uuid.UUID
}

type Channel struct {
	ID                  uuid.UUID
	OrganizationID      uuid.UUID
	ProjectID           *uuid.UUID
	Kind                string
	Name                string
	Locale              string
	Enabled             bool
	DisabledReason      string
	ConsecutiveFailures int
	Host                string
	HasSecret           bool
	Version             int64
	CreatedByUserID     uuid.UUID
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type ChannelPage struct {
	Items []Channel
	Total int64
}

type CreateChannelInput struct {
	Kind     string
	Name     string
	Locale   string
	URL      string
	BotToken string
	ChatID   string
}

type UpdateChannelInput struct {
	Name         string
	Locale       string
	Version      int64
	URL          *string
	BotToken     *string
	ChatID       *string
	RotateSecret bool
	Enable       bool
}

type CreatedChannel struct {
	Channel Channel
	Secret  string
}

type Rule struct {
	ChannelID    uuid.UUID
	Events       []string
	Environments []string
	Statuses     []string
	Roles        []string
	Version      int64
}

type Delivery struct {
	ID            uuid.UUID
	ChannelID     uuid.UUID
	ChannelName   string
	ChannelKind   string
	Event         string
	EventID       uuid.UUID
	State         string
	Attempts      int
	NextAttemptAt time.Time
	LastAttemptAt *time.Time
	SentAt        *time.Time
	LastError     string
	LastStatus    *int
	CreatedAt     time.Time
}

type DeliveryPage struct {
	Items []Delivery
	Total int64
}

package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

type Kind string

const (
	KindEmail         Kind = "email"
	KindWebhook       Kind = "webhook"
	KindSlack         Kind = "slack"
	KindTelegram      Kind = "telegram"
	KindDiscord       Kind = "discord"
	KindInApp         Kind = "inapp"
	KindInternalEmail Kind = "internal-email"

	PayloadMaxBytes = 16 << 10
	nameMax         = 128

	StatePending = "pending"
	StateSending = "sending"
	StateSent    = "sent"
	StateDead    = "dead"

	ErrLeaseExpired = "lease_expired"
	ErrNoAdapter    = "no_adapter"
)

var (
	ErrInvalidEntry = errors.New("the outbox entry is not valid")
	ErrNoClock      = errors.New("the outbox entry needs a time to first try")
)

type Entry struct {
	OrganizationID  uuid.UUID
	ProjectID       *uuid.UUID
	ChannelID       *uuid.UUID
	RecipientUserID *uuid.UUID
	Kind            Kind
	EventID         uuid.UUID
	EventName       string
	Payload         []byte
	NextAttemptAt   time.Time
}

func (e Entry) validate() error {
	switch {
	case e.EventID == uuid.Nil || (e.OrganizationID == uuid.Nil && e.Kind != KindInternalEmail):
		return fmt.Errorf("%w: the organization and the event need ids", ErrInvalidEntry)
	case e.EventName == "" || len(e.EventName) > nameMax:
		return fmt.Errorf("%w: the event name is empty or too long", ErrInvalidEntry)
	case e.NextAttemptAt.IsZero():
		return ErrNoClock
	case len(e.Payload) == 0 || len(e.Payload) > PayloadMaxBytes || !json.Valid(e.Payload) || e.Payload[0] != '{':
		return fmt.Errorf("%w: the payload must be a JSON object of at most %d bytes", ErrInvalidEntry, PayloadMaxBytes)
	case e.Kind == KindInternalEmail && e.RecipientUserID == nil:
		return fmt.Errorf("%w: an internal email needs a recipient", ErrInvalidEntry)
	case e.Kind != KindInternalEmail && e.ChannelID == nil:
		return fmt.Errorf("%w: a delivery needs a channel", ErrInvalidEntry)
	}
	return nil
}

func Enqueue(ctx context.Context, q *sqlc.Queries, e Entry) (bool, error) {
	if err := e.validate(); err != nil {
		return false, err
	}
	n, err := q.InsertOutbox(ctx, sqlc.InsertOutboxParams{
		OrganizationID: orgOf(e.OrganizationID), ProjectID: e.ProjectID, ChannelID: e.ChannelID, RecipientUserID: e.RecipientUserID,
		Kind: string(e.Kind), EventID: e.EventID, EventName: e.EventName, Payload: e.Payload, NextAttemptAt: e.NextAttemptAt,
	})
	if err != nil {
		return false, fmt.Errorf("write to the outbox: %w", err)
	}
	return n == 1, nil
}

func Requeue(ctx context.Context, q *sqlc.Queries, org, id uuid.UUID, now time.Time) (bool, error) {
	n, err := q.RequeueOutbox(ctx, sqlc.RequeueOutboxParams{ID: id, OrganizationID: &org, Now: now})
	if err != nil {
		return false, fmt.Errorf("requeue a dead delivery: %w", err)
	}
	return n == 1, nil
}

type Item struct {
	ID              uuid.UUID
	OrganizationID  *uuid.UUID
	ProjectID       *uuid.UUID
	ChannelID       *uuid.UUID
	RecipientUserID *uuid.UUID
	Kind            Kind
	EventID         uuid.UUID
	EventName       string
	Payload         []byte
	Attempts        int
}

func orgOf(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}

func itemOf(row sqlc.NotificationOutbox) Item {
	return Item{
		ID: row.Id, OrganizationID: row.OrganizationId, ProjectID: row.ProjectId, ChannelID: row.ChannelId,
		RecipientUserID: row.RecipientUserId, Kind: Kind(row.Kind), EventID: row.EventId, EventName: row.EventName,
		Payload: row.Payload, Attempts: int(row.Attempts),
	}
}

type Result struct {
	Code      string
	Status    int
	Permanent bool
}

func (r Result) OK() bool { return r.Code == "" }

type Deliverer interface {
	Deliver(ctx context.Context, item Item) Result
}

type Observer interface {
	Settled(ctx context.Context, item Item, res Result, final bool)
}

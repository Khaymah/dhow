package secretstore

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/google/uuid"

	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

const redacted = "[secret]"

var (
	ErrFork        = errors.New("a run from a fork never receives secrets")
	ErrTooMany     = errors.New("the job names more secrets than one job may receive")
	ErrTooLarge    = errors.New("the secrets of the job are larger than one job may receive")
	ErrMissingName = errors.New("a secret the job names does not exist or is not available to this run")
)

type Request struct {
	OrganizationID uuid.UUID
	ProjectID      uuid.UUID
	EnvironmentID  *uuid.UUID
	Names          []string
	PullRequest    bool
	Fork           bool
}

type CheckResult struct {
	Missing            []string
	NotForPullRequests []string
}

func (r CheckResult) OK() bool { return len(r.Missing) == 0 && len(r.NotForPullRequests) == 0 }

type Value []byte

func (Value) String() string               { return redacted }
func (Value) GoString() string             { return redacted }
func (Value) Format(f fmt.State, _ rune)   { _, _ = f.Write([]byte(redacted)) }
func (Value) LogValue() slog.Value         { return slog.StringValue(redacted) }
func (Value) MarshalJSON() ([]byte, error) { return []byte(strconv.Quote(redacted)), nil }
func (Value) MarshalText() ([]byte, error) { return []byte(redacted), nil }
func (v Value) Wipe()                      { clear(v) }

type Secret struct {
	Name    string
	Scope   string
	Version int32
	Value   Value
}

func Wipe(list []Secret) {
	for _, s := range list {
		s.Value.Wipe()
	}
}

type Store interface {
	Check(ctx context.Context, q *sqlc.Queries, r Request) (CheckResult, error)
	Resolve(ctx context.Context, r Request) ([]Secret, error)
}

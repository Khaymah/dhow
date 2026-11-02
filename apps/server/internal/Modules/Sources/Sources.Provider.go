package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Sources/Models"
)

var (
	ErrUnsupported    = errors.New("the provider does not support this")
	ErrSignature      = errors.New("the webhook signature is missing, wrong or too old")
	ErrDeliveryID     = errors.New("the webhook has no valid delivery id")
	ErrPayload        = errors.New("the webhook payload is not valid")
	ErrHostKeyPending = errors.New("the host key of the repository is not confirmed")
)

type WebhookRequest struct {
	Header http.Header
	Body   []byte
	Secret []byte
	Now    time.Time
}

type Proof struct {
	Format     string
	DeliveryID string
}

type Credential struct {
	Kind      string
	Secret    []byte
	KnownHost string
	URL       string
}

const redacted = "[redacted]"

func (c Credential) LogValue() slog.Value {
	return slog.GroupValue(slog.String("Kind", c.Kind), slog.String("KnownHost", c.KnownHost), slog.String("Secret", redacted))
}

func (c Credential) String() string {
	return "Credential{Kind: " + c.Kind + ", KnownHost: " + c.KnownHost + ", Secret: " + redacted + "}"
}

func (c Credential) Format(f fmt.State, _ rune) {
	_, _ = io.WriteString(f, c.String())
}

type redactedCredential struct {
	Kind      string `json:"Kind"`
	KnownHost string `json:"KnownHost"`
	Secret    string `json:"Secret"`
}

func (c Credential) MarshalJSON() ([]byte, error) {
	return json.Marshal(redactedCredential{Kind: c.Kind, KnownHost: c.KnownHost, Secret: redacted})
}

type StatusReport struct {
	Sha       string
	State     string
	TargetURL string
}

type Provider interface {
	Name() string
	VerifyWebhook(r WebhookRequest) (Proof, error)
	ParseEvent(r WebhookRequest, p Proof) (models.Event, error)
	FetchFile(ctx context.Context, repo models.Repository, sha, path string) ([]byte, error)
	ResolveRef(ctx context.Context, repo models.Repository, ref string) (string, error)
	MintCheckoutCredential(ctx context.Context, repo models.Repository) (Credential, error)
	ReportStatus(ctx context.Context, repo models.Repository, report StatusReport) error
}

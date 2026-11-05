package sources

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Sources/Models"
	"github.com/Khaymah/dhow/internal/gitsource"
	"github.com/Khaymah/dhow/internal/sshhostkey"
)

const credentialSSHKey = "ssh_key"

type deployKey struct {
	public string
	sealed []byte
}

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("read random bytes: %w", err)
	}
	return b, nil
}

func newHookID() (string, error) {
	b, err := randomBytes(config.WebhookHookIDBytes)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func newWebhookSecret() (string, error) {
	b, err := randomBytes(config.WebhookSecretBytes)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (s *Service) sealWebhookSecret(ctx context.Context, q *sqlc.Queries, project, repo uuid.UUID, secret string) ([]byte, error) {
	return s.webhooks.SealIn(ctx, q, keys.ProjectOwner(project), sealContext(contextWebhook, repo), []byte(secret))
}

func (s *Service) newDeployKey(ctx context.Context, q *sqlc.Queries, project, repo uuid.UUID) (deployKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return deployKey{}, fmt.Errorf("generate the deploy key: %w", err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return deployKey{}, fmt.Errorf("encode the deploy key: %w", err)
	}
	block, err := ssh.MarshalPrivateKey(priv, config.DeployKeyComment)
	if err != nil {
		return deployKey{}, fmt.Errorf("encode the deploy key: %w", err)
	}
	private := pem.EncodeToMemory(block)
	defer clear(private)
	sealed, err := s.deployKeys.SealIn(ctx, q, keys.ProjectOwner(project), sealContext(contextDeploy, repo), private)
	if err != nil {
		return deployKey{}, err
	}
	public := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub))) + " " + config.DeployKeyComment
	return deployKey{public: public, sealed: sealed}, nil
}

func fingerprintOf(authorized string) string { return sshhostkey.Fingerprint(authorized) }

func authorizedLine(key ssh.PublicKey) string { return sshhostkey.AuthorizedLine(key) }

func (s *Service) deployKeyFor(ctx context.Context, repo models.Repository) ([]byte, error) {
	row, err := s.queries().GetRepositoryByProject(ctx, sqlc.GetRepositoryByProjectParams{
		ProjectID: repo.ProjectID, OrganizationID: repo.OrganizationID,
	})
	if err != nil {
		return nil, err
	}
	if row.Id != repo.ID {
		return nil, errors.New("the repository changed")
	}
	return s.openDeployKey(ctx, row.ProjectId, row.Id, row.DeployKeySealed)
}

type keyOpener interface {
	deployKeyFor(ctx context.Context, repo models.Repository) ([]byte, error)
}

type genericProvider struct {
	keys keyOpener
}

func (genericProvider) Name() string { return models.ProviderGeneric }

func (genericProvider) VerifyWebhook(r WebhookRequest) (Proof, error) { return VerifyGenericWebhook(r) }

func (genericProvider) ParseEvent(r WebhookRequest, p Proof) (models.Event, error) {
	return ParseGenericEvent(r, p)
}

func (genericProvider) FetchFile(context.Context, models.Repository, string, string) ([]byte, error) {
	return nil, ErrUnsupported
}

func (genericProvider) ResolveRef(context.Context, models.Repository, string) (string, error) {
	return "", ErrUnsupported
}

func (g genericProvider) MintCheckoutCredential(ctx context.Context, repo models.Repository) (Credential, error) {
	if repo.Transport != models.TransportSSH {
		return Credential{}, nil
	}
	if repo.HostKey == "" {
		return Credential{}, ErrHostKeyPending
	}
	u, err := gitsource.Parse(repo.CloneURL)
	if err != nil {
		return Credential{}, err
	}
	private, err := g.keys.deployKeyFor(ctx, repo)
	if err != nil {
		return Credential{}, err
	}
	return Credential{Kind: credentialSSHKey, Secret: private, KnownHost: u.KnownHostsPattern() + " " + repo.HostKey, URL: u.String()}, nil
}

func (genericProvider) ReportStatus(context.Context, models.Repository, StatusReport) error {
	return ErrUnsupported
}

func expiresAfter(now time.Time) *time.Time {
	t := now.Add(config.DeployKeyRotationGrace)
	return &t
}

func (s *Service) CheckoutCredential(ctx context.Context, org, project, repo uuid.UUID) (Credential, error) {
	row, err := s.queries().GetRepositoryByProject(ctx, sqlc.GetRepositoryByProjectParams{ProjectID: project, OrganizationID: org})
	if err != nil {
		return Credential{}, err
	}
	if row.Id != repo {
		return Credential{}, errors.New("the repository of the run is no longer linked to its project")
	}
	p, ok := s.providers[row.Provider]
	if !ok {
		return Credential{}, ErrUnsupported
	}
	return p.MintCheckoutCredential(ctx, s.repositoryOf(row))
}

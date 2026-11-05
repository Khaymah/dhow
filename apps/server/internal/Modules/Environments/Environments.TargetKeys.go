package environments

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	ratelimit "github.com/Khaymah/dhow/apps/server/internal/Core/RateLimit"
	secretstore "github.com/Khaymah/dhow/apps/server/internal/Core/SecretStore"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Environments/Models"
	"github.com/Khaymah/dhow/internal/netguard"
	"github.com/Khaymah/dhow/internal/sshhostkey"
)

const (
	limitDialUser = "target-dial-user"
	limitDialOrg  = "target-dial-org"
	maxSecretName = 64
	hashChars     = 8
)

var ErrNoSecretWriter = errors.New("environments: the secret writer is not connected")

func KeySecretName(target string) string {
	base := strings.ToUpper(strings.ReplaceAll(target, "-", "_"))
	if base[0] >= '0' && base[0] <= '9' || strings.HasPrefix(base+config.TargetKeySuffix, "DHOW_") {
		base = "T_" + base
	}
	room := maxSecretName - len(config.TargetKeySuffix)
	if len(base) > room {
		sum := sha256.Sum256([]byte(target))
		base = base[:room-hashChars-1] + "_" + strings.ToUpper(hex.EncodeToString(sum[:])[:hashChars])
	}
	return base + config.TargetKeySuffix
}

type deployKey struct {
	private []byte
	public  string
}

func newDeployKey() (deployKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return deployKey{}, fmt.Errorf("generate the deploy key: %w", err)
	}
	defer clear(priv)
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return deployKey{}, fmt.Errorf("encode the deploy key: %w", err)
	}
	block, err := ssh.MarshalPrivateKey(priv, config.TargetKeyComment)
	if err != nil {
		return deployKey{}, fmt.Errorf("encode the deploy key: %w", err)
	}
	return deployKey{
		private: pem.EncodeToMemory(block),
		public:  strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub))) + " " + config.TargetKeyComment,
	}, nil
}

func (s *Service) GenerateKey(ctx context.Context, org, project uuid.UUID, envName, name string, factor auth.Factor) (models.Target, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.Target{}, err
	}
	if s.secrets == nil {
		return models.Target{}, ErrNoSecretWriter
	}
	if err := s.confirm(ctx, p, factor); err != nil {
		return models.Target{}, err
	}
	key, err := newDeployKey()
	if err != nil {
		return models.Target{}, err
	}
	defer clear(key.private)
	var out models.Target
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		locked, err := s.lockTarget(ctx, q, org, project, envName, name)
		if err != nil {
			return err
		}
		secretName := KeySecretName(locked.target.Name)
		taken, err := q.EnvironmentSecretExists(ctx, sqlc.EnvironmentSecretExistsParams{
			OrganizationID: org, EnvironmentID: &locked.env.ID, Name: secretName,
		})
		if err != nil {
			return err
		}
		if taken && !strings.EqualFold(locked.target.KeySecretName, secretName) {
			return apperrors.TargetKeyNameTaken()
		}
		written, err := s.secrets.PutEnvironmentSecret(ctx, q, secretstore.EnvironmentSecret{
			OrganizationID: org, ProjectID: project, EnvironmentID: locked.env.ID, Name: secretName,
			Description: "Deploy key of the target " + locked.target.Name, Value: secretstore.Value(key.private),
		})
		if err != nil {
			return err
		}
		row, err := q.SetDeployTargetKey(ctx, sqlc.SetDeployTargetKeyParams{
			ID: locked.row.Id, OrganizationID: org, KeySecretName: textOf(secretName), PublicKey: textOf(key.public), Now: s.now(),
		})
		if err != nil {
			return err
		}
		if out, err = s.oneTarget(ctx, q, row); err != nil {
			return err
		}
		return s.auditTarget(ctx, q, p, eventbus.TargetKeyGenerated, locked.env, out, map[string]string{
			"KeyName": secretName, "PreviousKeyName": locked.target.KeySecretName, "KeyVersion": fmt.Sprint(written.Version),
			"Fingerprint": fingerprintOfPublic(key.public), "Version": fmt.Sprint(out.Version),
		})
	})
	return out, err
}

func fingerprintOfPublic(authorized string) string { return sshhostkey.Fingerprint(authorized) }

func (s *Service) chargeDial(ctx context.Context, org, user uuid.UUID) error {
	if s.limiter == nil {
		return nil
	}
	for _, c := range []struct {
		limit   ratelimit.Limit
		subject string
	}{
		{ratelimit.Limit{Name: limitDialUser, Max: config.TargetDialPerUser, Window: config.TargetDialWindow}, user.String()},
		{ratelimit.Limit{Name: limitDialOrg, Max: config.TargetDialPerOrg, Window: config.TargetDialWindow}, org.String()},
	} {
		d, err := s.limiter.Take(ctx, c.limit, c.subject)
		if err != nil {
			return err
		}
		if err := d.Err(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) FetchHostKey(ctx context.Context, org, project uuid.UUID, envName, name string, hostID uuid.UUID) (models.Target, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.Target{}, err
	}
	t, err := s.GetTarget(ctx, org, project, envName, name)
	if err != nil {
		return models.Target{}, err
	}
	host, ok := findHost(t, hostID)
	if !ok {
		return models.Target{}, apperrors.NotFound()
	}
	if err := s.checkProject(ctx, s.queries(), org, project); err != nil {
		return models.Target{}, err
	}
	if err := s.chargeDial(ctx, org, p.UserID); err != nil {
		return models.Target{}, err
	}
	key, err := sshhostkey.Fetch(ctx, s.dialer, net.JoinHostPort(host.Host, strconv.Itoa(host.Port)), t.Username, config.HostKeyFetchTimeout)
	if errors.Is(err, netguard.ErrBlocked) {
		return models.Target{}, apperrors.TargetHostBlocked()
	}
	if err != nil {
		return models.Target{}, apperrors.TargetHostUnreachable(err)
	}
	return s.storePendingKey(ctx, p, pendingKey{
		org: org, project: project, envName: envName, name: name, hostID: hostID, address: addressOf(host.Host, host.Port),
		key: key, source: models.HostKeySourceFetched,
	})
}

func (s *Service) SetPendingHostKey(ctx context.Context, org, project uuid.UUID, envName, name string, hostID uuid.UUID, pasted string) (models.Target, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.Target{}, err
	}
	key, err := sshhostkey.Parse(pasted)
	if err != nil {
		return models.Target{}, apperrors.ValidationFailed(invalid(fieldHostKey))
	}
	return s.storePendingKey(ctx, p, pendingKey{
		org: org, project: project, envName: envName, name: name, hostID: hostID, key: key, source: models.HostKeySourcePasted,
	})
}

type pendingKey struct {
	org, project  uuid.UUID
	envName, name string
	hostID        uuid.UUID
	address       string
	key           ssh.PublicKey
	source        string
}

func (s *Service) storePendingKey(ctx context.Context, p auth.Principal, in pendingKey) (models.Target, error) {
	var out models.Target
	err := dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		locked, err := s.lockTarget(ctx, q, in.org, in.project, in.envName, in.name)
		if err != nil {
			return err
		}
		row, err := q.LockDeployTargetHost(ctx, sqlc.LockDeployTargetHostParams{ID: in.hostID, TargetID: locked.row.Id, OrganizationID: in.org})
		if err != nil {
			return notFoundOr(err)
		}
		if in.address != "" && in.address != addressOf(row.Host, int(row.Port)) {
			return apperrors.TargetVersionConflict()
		}
		line, fingerprint := sshhostkey.AuthorizedLine(in.key), ssh.FingerprintSHA256(in.key)
		pending := sqlc.SetDeployTargetHostPendingKeyParams{
			ID: in.hostID, OrganizationID: in.org, Now: s.now(), PendingHostKey: textOf(line), PendingFingerprint: textOf(fingerprint),
		}
		if row.HostKey.String == line {
			pending.PendingHostKey, pending.PendingFingerprint = textOf(""), textOf("")
		}
		if _, err := q.SetDeployTargetHostPendingKey(ctx, pending); err != nil {
			return err
		}
		if out, err = s.oneTarget(ctx, q, locked.row); err != nil {
			return err
		}
		return s.auditTarget(ctx, q, p, eventbus.TargetHostKeyFetched, locked.env, out, map[string]string{
			"HostId": in.hostID.String(), "Address": addressOf(row.Host, int(row.Port)), "Source": in.source, "Fingerprint": fingerprint,
			"PinnedFingerprint": row.HostKeyFingerprint.String,
		})
	})
	return out, err
}

func (s *Service) ConfirmHostKey(ctx context.Context, org, project uuid.UUID, envName, name string, hostID uuid.UUID, fingerprint string, factor auth.Factor) (models.Target, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.Target{}, err
	}
	if err := s.confirm(ctx, p, factor); err != nil {
		return models.Target{}, err
	}
	var out models.Target
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		locked, err := s.lockTarget(ctx, q, org, project, envName, name)
		if err != nil {
			return err
		}
		row, err := q.LockDeployTargetHost(ctx, sqlc.LockDeployTargetHostParams{ID: hostID, TargetID: locked.row.Id, OrganizationID: org})
		if err != nil {
			return notFoundOr(err)
		}
		if !row.PendingHostKey.Valid {
			return apperrors.HostKeyNotPending()
		}
		if row.PendingFingerprint.String != fingerprint {
			return apperrors.HostKeyMismatch()
		}
		actor := p.UserID
		if _, err := q.ConfirmDeployTargetHostKey(ctx, sqlc.ConfirmDeployTargetHostKeyParams{
			ID: hostID, OrganizationID: org, ConfirmedBy: &actor, Now: s.now(),
		}); err != nil {
			return err
		}
		if out, err = s.touchedTarget(ctx, q, locked.row, s.now()); err != nil {
			return err
		}
		return s.auditTarget(ctx, q, p, eventbus.TargetHostKeyConfirmed, locked.env, out, map[string]string{
			"HostId": hostID.String(), "Address": addressOf(row.Host, int(row.Port)), "Position": fmt.Sprint(row.Position),
			"Fingerprint": fingerprint, "PreviousFingerprint": row.HostKeyFingerprint.String, "Version": fmt.Sprint(out.Version),
		})
	})
	return out, err
}

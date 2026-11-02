package runners

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	runnerauth "github.com/Khaymah/dhow/apps/server/internal/Core/RunnerAuth"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Runners/Models"
	"github.com/Khaymah/dhow/internal/runnersig"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

var errCloneSuspected = errors.New("runner identity is used from more than one place")

func (s *Service) Challenge(_ context.Context, runnerID string) ([]byte, []byte, error) {
	id, err := uuid.Parse(runnerID)
	if err != nil {
		return nil, nil, runnerauth.ValidationFailed("Runner")
	}
	nonce := make([]byte, runnersig.NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, fmt.Errorf("read random bytes: %w", err)
	}
	challenge, err := proto.MarshalOptions{Deterministic: true}.Marshal(&runnerv1.AuthChallenge{
		Nonce: nonce, RunnerId: id.String(), ExpiresAt: timestamppb.New(s.now().Add(config.RunnerChallengeTTL)),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("encode challenge: %w", err)
	}
	return challenge, s.keys.ChallengeMAC(challenge), nil
}

func (s *Service) openChallenge(in models.AuthenticateInput, now time.Time) (uuid.UUID, []byte, bool) {
	if !s.keys.VerifyChallengeMAC(in.ChallengeBytes, in.ChallengeMAC) {
		return uuid.Nil, nil, false
	}
	var c runnerv1.AuthChallenge
	if err := proto.Unmarshal(in.ChallengeBytes, &c); err != nil || len(c.GetNonce()) != runnersig.NonceSize || c.GetExpiresAt() == nil {
		return uuid.Nil, nil, false
	}
	id, err := uuid.Parse(c.GetRunnerId())
	if err != nil || c.GetRunnerId() != in.RunnerID {
		return uuid.Nil, nil, false
	}
	expires := c.GetExpiresAt().AsTime()
	if !now.Before(expires) || expires.Sub(now) > config.RunnerChallengeTTL {
		return uuid.Nil, nil, false
	}
	return id, c.GetNonce(), true
}

func identityError(state, reason string) error {
	switch {
	case state == models.StateRevoked:
		return runnerauth.RunnerRevoked()
	case state == models.StateDisabled && reason == models.DisabledCloneSuspected:
		return runnerauth.RunnerDisabled()
	default:
		return nil
	}
}

func (s *Service) Authenticate(ctx context.Context, in models.AuthenticateInput) (string, error) {
	meta := runnerauth.RequestMetaFrom(ctx)
	now := s.now()
	key := authenticateKey(meta.IPString())
	wait, err := s.reserve(ctx, now, key)
	if err != nil {
		return "", err
	}
	if wait > 0 {
		return "", runnerauth.InvalidRunnerCredentials(wait)
	}
	runnerID, nonce, ok := s.openChallenge(in, now)
	if !ok {
		return "", runnerauth.InvalidRunnerCredentials(0)
	}
	ident, err := s.queries().GetRunnerIdentity(ctx, runnerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", runnerauth.InvalidRunnerCredentials(0)
	}
	if err != nil {
		return "", err
	}
	features, err := runnersig.FeatureNames(in.Features)
	if err != nil {
		return "", runnerauth.InvalidRunnerCredentials(0)
	}
	message := runnersig.AuthMessage(s.origin, s.keys.SigningKeyVersion(), in.ChallengeBytes)
	if len(features) > 0 {
		message = runnersig.AuthFeaturesMessage(s.origin, s.keys.SigningKeyVersion(), in.ChallengeBytes, features)
	}
	if !runnersig.Verify(ident.SigningPublicKey, message, in.Signature) {
		return "", runnerauth.InvalidRunnerCredentials(0)
	}
	if err := identityError(ident.State, ident.DisabledReason); err != nil {
		return "", err
	}
	token, hash, err := runnerauth.NewAccessToken()
	if err != nil {
		return "", err
	}
	switches, err := s.openSession(ctx, now, session{signingKey: ident.SigningPublicKey, runnerID: runnerID, nonce: nonce, hash: hash, features: features})
	if errors.Is(err, errCloneSuspected) {
		return "", s.reportClone(ctx, ident.OrganizationId, runnerID, switches)
	}
	if err != nil {
		return "", err
	}
	s.refund(ctx, now, key)
	return token, nil
}

type session struct {
	signingKey []byte
	runnerID   uuid.UUID
	nonce      []byte
	hash       []byte
	features   []string
}

func (s *Service) openSession(ctx context.Context, now time.Time, in session) (int64, error) {
	signingKey, runnerID, nonce, hash := in.signingKey, in.runnerID, in.nonce, in.hash
	var switches int64
	clone := false
	err := dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		ident, err := q.LockRunnerIdentity(ctx, runnerID)
		if err != nil {
			return err
		}
		if err := identityError(ident.State, ident.DisabledReason); err != nil {
			return err
		}
		if !bytes.Equal(ident.SigningPublicKey, signingKey) {
			return runnerauth.InvalidRunnerCredentials(0)
		}
		if err := q.SetRunnerFeatures(ctx, sqlc.SetRunnerFeaturesParams{ID: runnerID, Features: in.features, Now: now}); err != nil {
			return err
		}
		var replaced *uuid.UUID
		prev, err := q.GetActiveRunnerSession(ctx, runnerID)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
		case err != nil:
			return err
		default:
			if err := q.RevokeRunnerSession(ctx, sqlc.RevokeRunnerSessionParams{ID: prev.Id, Now: now}); err != nil {
				return err
			}
			if prev.ExpiresAt.After(now) {
				replaced = &prev.Id
			}
		}
		_, err = q.InsertRunnerSession(ctx, sqlc.InsertRunnerSessionParams{
			RunnerID: runnerID, TokenHash: hash, ChallengeNonce: nonce, KeyVersion: ident.KeyVersion,
			ExpiresAt: now.Add(config.RunnerAccessTokenTTL), ReplacedSessionID: replaced,
			IpAddress: runnerauth.RequestMetaFrom(ctx).IPString(), Now: now,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return runnerauth.InvalidRunnerCredentials(0)
		}
		if err != nil {
			return err
		}
		if replaced != nil {
			if switches, err = q.CountRunnerSessionSwitches(ctx, sqlc.CountRunnerSessionSwitchesParams{
				RunnerID: runnerID, Since: now.Add(-config.RunnerCloneWindow),
			}); err != nil {
				return err
			}
		}
		if switches > config.RunnerCloneMaxSwitches {
			clone = true
			if _, err := q.DisableRunnerAsClone(ctx, sqlc.DisableRunnerAsCloneParams{ID: runnerID, Now: now}); err != nil {
				return err
			}
			_, err = q.RevokeRunnerSessions(ctx, sqlc.RevokeRunnerSessionsParams{RunnerID: runnerID, Now: now})
			return err
		}
		return q.TouchRunner(ctx, sqlc.TouchRunnerParams{
			ID: runnerID, Now: now, IpAddress: runnerauth.RequestMetaFrom(ctx).IPString(), TouchBefore: now,
		})
	})
	if err == nil && clone {
		return switches, errCloneSuspected
	}
	return switches, err
}

func (s *Service) reportClone(ctx context.Context, org, runnerID uuid.UUID, switches int64) error {
	s.log.WarnContext(ctx, "runner disabled: its identity is used from more than one place", "runner_id", runnerID.String(), "switches", switches)
	err := s.bus.Publish(ctx, eventbus.Event{Name: eventbus.RunnerDisabledCloneSuspected, Payload: eventbus.Audited{
		OrganizationIDs: []uuid.UUID{org}, TargetType: "runner", TargetID: runnerID.String(),
		IP: runnerauth.RequestMetaFrom(ctx).IPString(), Data: map[string]string{
			"Switches": strconv.FormatInt(switches, 10), "Window": config.RunnerCloneWindow.String(),
		},
	}})
	if err != nil {
		return err
	}
	return runnerauth.RunnerDisabled()
}

func (s *Service) RotateKeys(ctx context.Context, in models.RotateInput) (int32, error) {
	p, ok := runnerauth.PrincipalFrom(ctx)
	if !ok {
		return 0, runnerauth.Unauthenticated()
	}
	newSigning, err := runnersig.ParseSigningKey(in.NewSigningPublicKey)
	if err != nil || bytes.Equal(in.NewSigningPublicKey, p.SigningPublicKey) {
		return 0, runnerauth.ValidationFailed("NewSigningPublicKey")
	}
	if _, err := runnersig.ParseEncryptionKey(in.NewEncryptionPublicKey); err != nil {
		return 0, runnerauth.ValidationFailed("NewEncryptionPublicKey")
	}
	message := runnersig.RotateMessage(s.origin, p.RunnerID.String(), p.KeyVersion, in.NewSigningPublicKey, in.NewEncryptionPublicKey)
	if !runnersig.Verify(p.SigningPublicKey, message, in.OldKeySignature) || !runnersig.Verify(newSigning, message, in.NewKeySignature) {
		return 0, runnerauth.InvalidRunnerCredentials(0)
	}
	now := s.now()
	var version int32
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		v, err := q.RotateRunnerKeys(ctx, sqlc.RotateRunnerKeysParams{
			ID: p.RunnerID, KeyVersion: p.KeyVersion, SigningPublicKey: in.NewSigningPublicKey,
			EncryptionPublicKey: in.NewEncryptionPublicKey, Now: now,
		})
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return runnerauth.KeyVersionChanged()
		case isKeyInUse(err):
			return runnerauth.KeyInUse()
		case err != nil:
			return err
		}
		version = v
		_, err = q.RevokeRunnerSessions(ctx, sqlc.RevokeRunnerSessionsParams{RunnerID: p.RunnerID, Now: now})
		return err
	})
	if err != nil {
		return 0, err
	}
	err = s.bus.Publish(ctx, eventbus.Event{Name: eventbus.RunnerKeysRotated, Payload: eventbus.Audited{
		OrganizationIDs: []uuid.UUID{p.OrganizationID}, TargetType: "runner", TargetID: p.RunnerID.String(),
		IP: runnerauth.RequestMetaFrom(ctx).IPString(), Data: map[string]string{"KeyVersion": strconv.Itoa(int(version))},
	}})
	if err != nil {
		return 0, err
	}
	return version, nil
}

func (s *Service) RevokeSelf(ctx context.Context, signature []byte) error {
	p, ok := runnerauth.PrincipalFrom(ctx)
	if !ok {
		return runnerauth.Unauthenticated()
	}
	if len(signature) != ed25519.SignatureSize {
		return runnerauth.ValidationFailed("Signature")
	}
	if !runnersig.Verify(p.SigningPublicKey, runnersig.RevokeMessage(s.origin, p.RunnerID.String(), p.KeyVersion), signature) {
		return runnerauth.InvalidRunnerCredentials(0)
	}
	now := s.now()
	var row sqlc.Runner
	revoked := false
	err := dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		r, err := q.RevokeRunner(ctx, sqlc.RevokeRunnerParams{ID: p.RunnerID, OrganizationID: p.OrganizationID, Now: now})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		row, revoked = r, true
		_, err = q.RevokeRunnerSessions(ctx, sqlc.RevokeRunnerSessionsParams{RunnerID: p.RunnerID, Now: now})
		return err
	})
	if err != nil || !revoked {
		return err
	}
	return s.bus.Publish(ctx, eventbus.Event{Name: eventbus.RunnerRevoked, Payload: eventbus.Audited{
		OrganizationIDs: []uuid.UUID{p.OrganizationID}, TargetType: "runner", TargetID: p.RunnerID.String(),
		IP: runnerauth.RequestMetaFrom(ctx).IPString(), Data: map[string]string{"Name": row.Name, "Reason": "revoked_by_runner"},
	}})
}

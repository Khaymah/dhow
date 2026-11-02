package runners

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	runnerauth "github.com/Khaymah/dhow/apps/server/internal/Core/RunnerAuth"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Runners/Models"
	"github.com/Khaymah/dhow/internal/runnersig"
)

const (
	uniqueViolation     = "23505"
	signingKeyIndexName = "UX_Runners_SigningPublicKey"
)

func isKeyInUse(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == signingKeyIndexName
}

func (s *Service) checkRegistration(in *models.RegisterInput) error {
	if field := validRegistration(in); field != "" {
		return runnerauth.ValidationFailed(field)
	}
	signing, err := runnersig.ParseSigningKey(in.SigningPublicKey)
	if err != nil {
		return runnerauth.ValidationFailed("SigningPublicKey")
	}
	if _, err := runnersig.ParseEncryptionKey(in.EncryptionPublicKey); err != nil {
		return runnerauth.ValidationFailed("EncryptionPublicKey")
	}
	if !runnersig.Verify(signing, runnersig.RegisterMessage(s.origin, in.SigningPublicKey, in.EncryptionPublicKey), in.Proof) {
		return runnerauth.ValidationFailed("Proof")
	}
	return nil
}

func (s *Service) Register(ctx context.Context, in models.RegisterInput) (models.RegisterResult, error) {
	meta := runnerauth.RequestMetaFrom(ctx)
	now := s.now().UTC().Truncate(timePrecision)
	key := registerKey(meta.IPString())
	wait, err := s.reserve(ctx, now, key)
	if err != nil {
		return models.RegisterResult{}, err
	}
	if wait > 0 {
		return models.RegisterResult{}, runnerauth.InvalidRegistrationToken(wait)
	}
	raw, ok := runnerauth.ParseRegistrationToken(in.Token)
	if !ok {
		return models.RegisterResult{}, runnerauth.InvalidRegistrationToken(0)
	}
	if err := s.checkRegistration(&in); err != nil {
		return models.RegisterResult{}, err
	}
	features, err := runnersig.FeatureNames(in.Features)
	if err != nil {
		return models.RegisterResult{}, runnerauth.ValidationFailed("Features")
	}
	var row sqlc.Runner
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		token, err := q.ConsumeRegistrationToken(ctx, sqlc.ConsumeRegistrationTokenParams{TokenHash: s.keys.RegistrationTokenHash(raw), Now: now})
		if errors.Is(err, pgx.ErrNoRows) {
			return runnerauth.InvalidRegistrationToken(0)
		}
		if err != nil {
			return err
		}
		row, err = q.InsertRunner(ctx, sqlc.InsertRunnerParams{
			OrganizationID: token.OrganizationId, RegistrationTokenID: token.Id, Name: in.Name, Labels: token.Labels,
			CapabilityLabels: capabilityLabels(in.Os, in.Arch, in.IsolationLevels), Os: in.Os, Arch: in.Arch, Version: in.Version,
			IsolationLevels: in.IsolationLevels, Capacity: int32(in.Capacity), //nolint:gosec // Bounded to 1..16 by validRegistration.
			SigningPublicKey: in.SigningPublicKey, EncryptionPublicKey: in.EncryptionPublicKey, IpAddress: meta.IPString(), Now: now,
			Features: features,
		})
		if isKeyInUse(err) {
			return runnerauth.KeyInUse()
		}
		return err
	})
	if err != nil {
		return models.RegisterResult{}, err
	}
	s.refund(ctx, now, key)
	err = s.bus.Publish(ctx, eventbus.Event{Name: eventbus.RunnerRegistered, Payload: eventbus.Audited{
		OrganizationIDs: []uuid.UUID{row.OrganizationId}, TargetType: "runner", TargetID: row.Id.String(), IP: meta.IPString(),
		Data: map[string]string{
			"Name": row.Name, "Os": row.Os, "Arch": row.Arch, "Version": row.Version, "Labels": strings.Join(row.Labels, ","),
			"RegisteredWith": row.RegistrationTokenId.String(),
		},
	}})
	if err != nil {
		s.revokeUnaudited(ctx, row)
		return models.RegisterResult{}, err
	}
	return models.RegisterResult{
		RunnerID: row.Id, OrganizationID: row.OrganizationId, Labels: append(append([]string{}, row.Labels...), row.CapabilityLabels...),
		ServerSigningKey: s.keys.SigningPublicKey(), ServerKeyVersion: s.keys.SigningKeyVersion(),
	}, nil
}

func (s *Service) revokeUnaudited(ctx context.Context, row sqlc.Runner) {
	_, err := s.queries().RevokeRunner(context.WithoutCancel(ctx), sqlc.RevokeRunnerParams{ID: row.Id, OrganizationID: row.OrganizationId, Now: s.now()})
	if err != nil {
		s.log.ErrorContext(ctx, "revoke runner after audit failure", "error", err)
	}
}

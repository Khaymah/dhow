package session

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Session/Models"
)

const totpContextLabel = "totp:"

func totpContext(userID uuid.UUID) []byte { return []byte(totpContextLabel + userID.String()) }

func (s *Service) verifyTotp(ctx context.Context, q *sqlc.Queries, userID uuid.UUID, code string, now time.Time) (bool, error) {
	if len(code) > config.MfaCodeMaxBytes {
		return false, nil
	}
	row, err := q.GetConfirmedTotp(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	secret, err := s.totp.Open(ctx, keys.UserOwner(userID), totpContext(userID), row.SecretSealed)
	if err != nil {
		return false, err
	}
	defer clear(secret)
	last := auth.NoTotpStep
	if row.LastUsedStep.Valid {
		last = row.LastUsedStep.Int64
	}
	step, ok := auth.MatchTotp(secret, code, now, last)
	if !ok {
		return false, nil
	}
	n, err := q.AdvanceTotpStep(ctx, sqlc.AdvanceTotpStepParams{ID: row.Id, Step: pgInt8(step), Now: now})
	return n == 1, err
}

func (s *Service) consumeRecoveryCode(ctx context.Context, q *sqlc.Queries, userID uuid.UUID, code string, now time.Time) (bool, error) {
	normalized, ok := auth.NormalizeRecoveryCode(code)
	if !ok {
		return false, nil
	}
	n, err := q.ConsumeRecoveryCode(ctx, sqlc.ConsumeRecoveryCodeParams{UserID: userID, CodeHash: s.recovery.Hash(normalized), Now: now})
	return n == 1, err
}

func (s *Service) factors(ctx context.Context, userID uuid.UUID) (sqlc.GetMfaFactorsRow, error) {
	return s.queries().GetMfaFactors(ctx, userID)
}

func (s *Service) confirmFactor(ctx context.Context, p auth.Principal, f models.Factor) error {
	if p.ViaToken() || p.SessionID == uuid.Nil {
		return apperrors.SessionRequired()
	}
	factors, err := s.factors(ctx, p.UserID)
	if err != nil {
		return err
	}
	if !hasMfa(factors) {
		if f.Method != models.MethodPassword {
			return apperrors.InvalidCurrentFactor(0)
		}
		return s.confirmPassword(ctx, p, f.Password)
	}
	if f.Method == models.MethodPassword {
		return apperrors.InvalidCurrentFactor(0)
	}
	now := s.now()
	key := mfaKey(p.UserID)
	wait, err := s.reserve(ctx, now, key)
	if err != nil {
		return err
	}
	if wait > 0 {
		return apperrors.InvalidCurrentFactor(wait)
	}
	var outcome factorOutcome
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		var err error
		outcome, err = s.checkCurrentFactor(ctx, q, p, f, now)
		return err
	})
	if err != nil {
		return err
	}
	if !outcome.ok {
		return s.failFactor(ctx, p.UserID, f.Method, "confirm", outcome, apperrors.InvalidCurrentFactor(0))
	}
	if err := s.resetFailures(ctx, now, key); err != nil {
		return err
	}
	return s.afterFactor(ctx, p.UserID, outcome)
}

func (s *Service) checkCurrentFactor(ctx context.Context, q *sqlc.Queries, p auth.Principal, f models.Factor, now time.Time) (factorOutcome, error) {
	if f.Method != models.MethodPasskey {
		return s.checkSecondFactor(ctx, q, p.UserID, models.MfaLoginInput{Method: f.Method, Code: f.Code}, nil, now)
	}
	out := factorOutcome{method: models.MethodPasskey}
	userID, sessionID := p.UserID, p.SessionID
	session, err := q.TakeWebauthnChallenge(ctx, sqlc.TakeWebauthnChallengeParams{
		ID: f.ChallengeID, Kind: kindPasskeyStepUp, UserID: &userID, SessionID: &sessionID, Now: now,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	user, err := s.loadPasskeyUser(ctx, q, p.UserID, false)
	if err != nil {
		return out, err
	}
	return s.verifyAssertion(ctx, q, user, session, f.Credential, now)
}

func (s *Service) confirmPassword(ctx context.Context, p auth.Principal, password string) error {
	if len(password) > maxPasswordInputBytes {
		return apperrors.InvalidCurrentFactor(0)
	}
	now := s.now()
	account := accountKey(auth.NormalizeEmail(p.Email))
	wait, err := s.reserve(ctx, now, account)
	if err != nil {
		return err
	}
	if wait > 0 {
		return apperrors.InvalidCurrentFactor(wait)
	}
	stored, err := s.queries().GetUserPasswordHash(ctx, p.UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.Unauthenticated()
	}
	if err != nil {
		return err
	}
	ok, _, err := s.hasher.Verify(ctx, password, stored)
	if err != nil {
		return err
	}
	if !ok {
		return s.failFactor(ctx, p.UserID, models.MethodPassword, "confirm", factorOutcome{}, apperrors.InvalidCurrentFactor(0))
	}
	return s.resetFailures(ctx, now, account)
}

func (s *Service) Status(ctx context.Context) (models.MfaStatus, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.MfaStatus{}, err
	}
	f, err := s.factors(ctx, p.UserID)
	if err != nil {
		return models.MfaStatus{}, err
	}
	out := models.MfaStatus{
		Enabled: hasMfa(f), TotpEnabled: f.HasTotp,
		RecoveryCodesRemaining: int(f.RecoveryCodesRemaining), PasskeyCount: int(f.PasskeyCount),
	}
	if f.HasTotp {
		row, err := s.queries().GetConfirmedTotp(ctx, p.UserID)
		if err != nil {
			return models.MfaStatus{}, err
		}
		out.TotpEnabledAt = row.ConfirmedAt
	}
	return out, nil
}

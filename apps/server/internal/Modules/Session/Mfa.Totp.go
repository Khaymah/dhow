package session

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Session/Models"
)

const (
	fieldCode          = "Code"
	messageInvalidCode = "errors.invalidMfaCode"
	messageTotpOff     = "errors.totpNotEnabled"
	messageMfaDisabled = "errors.mfaNotEnabled"
)

func pgInt8(v int64) pgtype.Int8 { return pgtype.Int8{Int64: v, Valid: true} }

func conflict(key string) *apperrors.AppError {
	return apperrors.New(http.StatusConflict, apperrors.CodeConflict, key)
}

func (s *Service) StartTotp(ctx context.Context) (models.TotpEnrollment, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.TotpEnrollment{}, err
	}
	secret, err := auth.NewTotpSecret()
	if err != nil {
		return models.TotpEnrollment{}, err
	}
	defer clear(secret)
	now := s.now()
	expires := now.Add(config.TotpEnrollmentTTL)
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		sealed, err := s.totp.SealIn(ctx, q, keys.UserOwner(p.UserID), totpContext(p.UserID), secret)
		if err != nil {
			return err
		}
		if err := q.DiscardPendingTotp(ctx, sqlc.DiscardPendingTotpParams{UserID: p.UserID, Now: now}); err != nil {
			return err
		}
		sessionID := p.SessionID
		_, err = q.InsertTotpEnrollment(ctx, sqlc.InsertTotpEnrollmentParams{
			UserID: p.UserID, SessionID: &sessionID, SecretSealed: sealed, ExpiresAt: expires, Now: now,
		})
		return err
	})
	if err != nil {
		return models.TotpEnrollment{}, err
	}
	return models.TotpEnrollment{
		Secret: auth.TotpSecretText(secret), URI: auth.TotpURI(config.TotpIssuer, p.Email, secret), ExpiresAt: expires,
	}, nil
}

func (s *Service) checkEnrollmentCode(ctx context.Context, q *sqlc.Queries, p auth.Principal, code string, lock bool) (uuid.UUID, int64, error) {
	now := s.now()
	sessionID := p.SessionID
	params := sqlc.LockPendingTotpParams{UserID: p.UserID, SessionID: &sessionID, Now: now}
	var row sqlc.LockPendingTotpRow
	var err error
	if lock {
		row, err = q.LockPendingTotp(ctx, params)
	} else {
		var peek sqlc.PeekPendingTotpRow
		peek, err = q.PeekPendingTotp(ctx, sqlc.PeekPendingTotpParams(params))
		row = sqlc.LockPendingTotpRow(peek)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, 0, apperrors.EnrollmentExpired()
	}
	if err != nil {
		return uuid.Nil, 0, err
	}
	secret, err := s.totp.Open(ctx, keys.UserOwner(p.UserID), totpContext(p.UserID), row.SecretSealed)
	if err != nil {
		return uuid.Nil, 0, err
	}
	defer clear(secret)
	if len(code) > config.MfaCodeMaxBytes {
		code = ""
	}
	step, ok := auth.MatchTotp(secret, code, now, auth.NoTotpStep)
	if !ok {
		return uuid.Nil, 0, apperrors.ValidationFailed(apperrors.IErrorDetail{Field: fieldCode, Message: messageInvalidCode})
	}
	return row.Id, step, nil
}

func (s *Service) ConfirmTotp(ctx context.Context, code string, factor models.Factor) (models.MfaChange, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.MfaChange{}, err
	}
	if _, _, err := s.checkEnrollmentCode(ctx, s.queries(), p, code, false); err != nil {
		return models.MfaChange{}, err
	}
	before, err := s.factors(ctx, p.UserID)
	if err != nil {
		return models.MfaChange{}, err
	}
	if err := s.confirmFactor(ctx, p, factor); err != nil {
		return models.MfaChange{}, err
	}
	now := s.now()
	var change models.MfaChange
	var replaced int64
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		id, step, err := s.checkEnrollmentCode(ctx, q, p, code, true)
		if err != nil {
			return err
		}
		if replaced, err = q.RetireConfirmedTotp(ctx, sqlc.RetireConfirmedTotpParams{UserID: p.UserID, Now: now}); err != nil {
			return err
		}
		if err := q.ConfirmTotpFactor(ctx, sqlc.ConfirmTotpFactorParams{ID: id, Step: pgInt8(step), Now: now}); err != nil {
			return err
		}
		if err := q.ClearMfaReset(ctx, sqlc.ClearMfaResetParams{ID: p.UserID, Now: now}); err != nil {
			return err
		}
		change, err = s.finishChange(ctx, q, p, !hasMfa(before))
		return err
	})
	if err != nil {
		return models.MfaChange{}, err
	}
	return change, s.publishTarget(ctx, eventbus.AuthTotpEnabled, p, "user", p.UserID.String(), map[string]string{
		"Replaced": strconv.FormatBool(replaced > 0), "RevokedSessions": itoa(change.Revoked),
	})
}

func (s *Service) DisableTotp(ctx context.Context, factor models.Factor) (models.MfaChange, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.MfaChange{}, err
	}
	before, err := s.factors(ctx, p.UserID)
	if err != nil {
		return models.MfaChange{}, err
	}
	if !before.HasTotp {
		return models.MfaChange{}, conflict(messageTotpOff)
	}
	if err := s.confirmFactor(ctx, p, factor); err != nil {
		return models.MfaChange{}, err
	}
	now := s.now()
	var change models.MfaChange
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		n, err := q.RetireConfirmedTotp(ctx, sqlc.RetireConfirmedTotpParams{UserID: p.UserID, Now: now})
		if err != nil {
			return err
		}
		if n == 0 {
			return conflict(messageTotpOff)
		}
		if err := s.dropRecoveryIfLast(ctx, q, p.UserID); err != nil {
			return err
		}
		change, err = s.finishChange(ctx, q, p, false)
		return err
	})
	if err != nil {
		return models.MfaChange{}, err
	}
	return change, s.publishTarget(ctx, eventbus.AuthTotpDisabled, p, "user", p.UserID.String(), map[string]string{"RevokedSessions": itoa(change.Revoked)})
}

func (s *Service) RegenerateRecoveryCodes(ctx context.Context, factor models.Factor) (models.MfaChange, error) {
	p, err := principalOf(ctx)
	if err != nil {
		return models.MfaChange{}, err
	}
	before, err := s.factors(ctx, p.UserID)
	if err != nil {
		return models.MfaChange{}, err
	}
	if !hasMfa(before) {
		return models.MfaChange{}, conflict(messageMfaDisabled)
	}
	if err := s.confirmFactor(ctx, p, factor); err != nil {
		return models.MfaChange{}, err
	}
	var change models.MfaChange
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		var err error
		change, err = s.finishChange(ctx, q, p, true)
		return err
	})
	if err != nil {
		return models.MfaChange{}, err
	}
	return change, s.publishTarget(ctx, eventbus.AuthRecoveryCodesRegenerated, p, "user", p.UserID.String(), map[string]string{"RevokedSessions": itoa(change.Revoked)})
}

func (s *Service) finishChange(ctx context.Context, q *sqlc.Queries, p auth.Principal, newCodes bool) (models.MfaChange, error) {
	now := s.now()
	var change models.MfaChange
	if newCodes {
		codes, err := s.replaceRecoveryCodes(ctx, q, p.UserID)
		if err != nil {
			return change, err
		}
		change.RecoveryCodes = codes
	}
	revoked, err := q.RevokeOtherUserSessions(ctx, sqlc.RevokeOtherUserSessionsParams{UserID: p.UserID, ExceptID: p.SessionID, Now: now})
	change.Revoked = revoked
	return change, err
}

func (s *Service) replaceRecoveryCodes(ctx context.Context, q *sqlc.Queries, userID uuid.UUID) ([]string, error) {
	now := s.now()
	if _, err := q.RetireRecoveryCodes(ctx, sqlc.RetireRecoveryCodesParams{UserID: userID, Now: now}); err != nil {
		return nil, err
	}
	codes, err := auth.NewRecoveryCodes(auth.RecoveryCodeCount)
	if err != nil {
		return nil, err
	}
	for _, c := range codes {
		normalized, ok := auth.NormalizeRecoveryCode(c)
		if !ok {
			return nil, errors.New("generated recovery code does not normalize")
		}
		if err := q.InsertRecoveryCode(ctx, sqlc.InsertRecoveryCodeParams{UserID: userID, CodeHash: s.recovery.Hash(normalized), Now: now}); err != nil {
			return nil, err
		}
	}
	return codes, nil
}

func (s *Service) dropRecoveryIfLast(ctx context.Context, q *sqlc.Queries, userID uuid.UUID) error {
	after, err := q.GetMfaFactors(ctx, userID)
	if err != nil || hasMfa(after) {
		return err
	}
	_, err = q.RetireRecoveryCodes(ctx, sqlc.RetireRecoveryCodesParams{UserID: userID, Now: s.now()})
	return err
}

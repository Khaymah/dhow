package preferences

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	dbtx "github.com/Khaymah/dhow/apps/server/internal/Database/Tx"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Preferences/Models"
)

type DB interface {
	sqlc.DBTX
	dbtx.Beginner
}

type Service struct {
	db  DB
	log *slog.Logger
	now func() time.Time
}

func NewService(db DB, log *slog.Logger, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{db: db, log: log, now: now}
}

func principal(ctx context.Context) (auth.Principal, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return auth.Principal{}, apperrors.Unauthenticated()
	}
	return p, nil
}

func (s *Service) Get(ctx context.Context) (models.Preferences, error) {
	p, err := principal(ctx)
	if err != nil {
		return models.Preferences{}, err
	}
	return s.load(ctx, sqlc.New(s.db), p)
}

func (s *Service) load(ctx context.Context, q *sqlc.Queries, p auth.Principal) (models.Preferences, error) {
	out, err := s.loadDisplay(ctx, q, p)
	if err != nil {
		return models.Preferences{}, err
	}
	seen, err := q.GetUserReleaseNotesSeen(ctx, p.UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return models.Preferences{}, err
	}
	out.ReleaseNotesSeen = &seen
	return out, nil
}

func (s *Service) loadDisplay(ctx context.Context, q *sqlc.Queries, p auth.Principal) (models.Preferences, error) {
	locale, err := q.GetUserLocale(ctx, p.UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Preferences{}, apperrors.Unauthenticated()
	}
	if err != nil {
		return models.Preferences{}, err
	}
	out := models.Preferences{Locale: locale, ThemeID: config.DefaultThemeID, CustomThemes: []models.CustomTheme{}}
	row, err := q.GetUserPreferences(ctx, p.UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return models.Preferences{}, err
	}
	if err := json.Unmarshal(row.CustomThemes, &out.CustomThemes); err != nil {
		return models.Preferences{}, fmt.Errorf("decode custom themes: %w", err)
	}
	updated := row.UpdatedAt
	out.ThemeID, out.Saved, out.UpdatedAt = row.ThemeId, true, &updated
	if row.OrganizationId != nil {
		member, err := q.IsActiveMember(ctx, sqlc.IsActiveMemberParams{UserID: p.UserID, OrganizationID: *row.OrganizationId})
		if err != nil {
			return models.Preferences{}, err
		}
		if member {
			out.OrganizationID = row.OrganizationId
		}
	}
	return out, nil
}

func (s *Service) Save(ctx context.Context, in models.Preferences) (models.Preferences, error) {
	p, err := principal(ctx)
	if err != nil {
		return models.Preferences{}, err
	}
	if in.CustomThemes == nil {
		in.CustomThemes = []models.CustomTheme{}
	}
	if ae := validate(in); ae != nil {
		return models.Preferences{}, ae
	}
	themes, err := json.Marshal(in.CustomThemes)
	if err != nil {
		return models.Preferences{}, fmt.Errorf("encode custom themes: %w", err)
	}
	var out models.Preferences
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		if in.OrganizationID != nil {
			member, err := q.IsActiveMember(ctx, sqlc.IsActiveMemberParams{UserID: p.UserID, OrganizationID: *in.OrganizationID})
			if err != nil {
				return err
			}
			if !member {
				return apperrors.ValidationFailed(apperrors.IErrorDetail{Field: fieldOrganization, Message: messageOrganization})
			}
		}
		now := s.now()
		if err := q.SetUserLocale(ctx, sqlc.SetUserLocaleParams{ID: p.UserID, Locale: in.Locale, Now: now}); err != nil {
			return err
		}
		if _, err := q.UpsertUserPreferences(ctx, sqlc.UpsertUserPreferencesParams{
			UserID: p.UserID, ThemeID: in.ThemeID, CustomThemes: themes, OrganizationID: in.OrganizationID, Now: now,
		}); err != nil {
			return err
		}
		out, err = s.load(ctx, q, p)
		return err
	})
	return out, err
}

func (s *Service) SaveReleaseNotesSeen(ctx context.Context, version string) (models.Preferences, error) {
	p, err := principal(ctx)
	if err != nil {
		return models.Preferences{}, err
	}
	if !versionPattern.MatchString(version) {
		return models.Preferences{}, apperrors.ValidationFailed(apperrors.IErrorDetail{Field: fieldVersion, Message: messageLocale})
	}
	var out models.Preferences
	err = dbtx.Run(ctx, s.db, func(q *sqlc.Queries) error {
		if _, err := q.UpsertUserReleaseNotesSeen(ctx, sqlc.UpsertUserReleaseNotesSeenParams{
			UserID: p.UserID, SeenVersion: version, Now: s.now(),
		}); err != nil {
			return err
		}
		out, err = s.load(ctx, q, p)
		return err
	})
	return out, err
}

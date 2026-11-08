package preferences

import (
	"context"

	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/Preferences/Gen"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Preferences/Models"
)

const (
	messageLoaded = "preferences.loaded"
	messageSaved  = "preferences.saved"
	messageSeen   = "preferences.releaseNotesSeen"
)

type Controller struct {
	service *Service
}

var _ gen.StrictServerInterface = (*Controller)(nil)

func NewController(service *Service) *Controller { return &Controller{service: service} }

func ptr[T any](v T) *T { return &v }

func toGen(p models.Preferences) *gen.Preferences {
	themes := make([]gen.CustomTheme, 0, len(p.CustomThemes))
	for _, t := range p.CustomThemes {
		themes = append(themes, gen.CustomTheme{Id: t.Id, Name: t.Name, Colors: t.Colors})
	}
	return &gen.Preferences{
		Locale: gen.Locale(p.Locale), ThemeId: p.ThemeID, CustomThemes: themes, OrganizationId: p.OrganizationID,
		Saved: p.Saved, UpdatedAt: p.UpdatedAt, ReleaseNotesSeen: p.ReleaseNotesSeen,
	}
}

func (c *Controller) GetPreferences(ctx context.Context, _ gen.GetPreferencesRequestObject) (gen.GetPreferencesResponseObject, error) {
	p, err := c.service.Get(ctx)
	if err != nil {
		return nil, err
	}
	return gen.GetPreferences200JSONResponse{Success: true, Message: ptr(messageLoaded), Data: toGen(p)}, nil
}

func (c *Controller) SavePreferences(ctx context.Context, req gen.SavePreferencesRequestObject) (gen.SavePreferencesResponseObject, error) {
	if req.Body == nil {
		return nil, apperrors.BadRequest()
	}
	b := req.Body
	themes := make([]models.CustomTheme, 0, len(b.CustomThemes))
	for _, t := range b.CustomThemes {
		themes = append(themes, models.CustomTheme{Id: t.Id, Name: t.Name, Colors: t.Colors})
	}
	p, err := c.service.Save(ctx, models.Preferences{
		Locale: string(b.Locale), ThemeID: b.ThemeId, CustomThemes: themes, OrganizationID: b.OrganizationId,
	})
	if err != nil {
		return nil, err
	}
	return gen.SavePreferences200JSONResponse{Success: true, Message: ptr(messageSaved), Data: toGen(p)}, nil
}

func (c *Controller) SaveReleaseNotesSeen(ctx context.Context, req gen.SaveReleaseNotesSeenRequestObject) (gen.SaveReleaseNotesSeenResponseObject, error) {
	if req.Body == nil {
		return nil, apperrors.BadRequest()
	}
	p, err := c.service.SaveReleaseNotesSeen(ctx, req.Body.Version)
	if err != nil {
		return nil, err
	}
	return gen.SaveReleaseNotesSeen200JSONResponse{Success: true, Message: ptr(messageSeen), Data: toGen(p)}, nil
}

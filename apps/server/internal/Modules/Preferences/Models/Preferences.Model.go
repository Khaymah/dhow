package models

import (
	"time"

	"github.com/google/uuid"
)

type CustomTheme struct {
	Id     string
	Name   string
	Colors map[string]string
}

type Preferences struct {
	Locale           string
	ThemeID          string
	CustomThemes     []CustomTheme
	OrganizationID   *uuid.UUID
	Saved            bool
	ReleaseNotesSeen *string
	UpdatedAt        *time.Time
}

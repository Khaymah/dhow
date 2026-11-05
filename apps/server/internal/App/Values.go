package app

import (
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	instancesettings "github.com/Khaymah/dhow/apps/server/internal/Core/Settings"
	secrets "github.com/Khaymah/dhow/apps/server/internal/Modules/Secrets"
	session "github.com/Khaymah/dhow/apps/server/internal/Modules/Session"
	sources "github.com/Khaymah/dhow/apps/server/internal/Modules/Sources"
)

func ValueRegistry() (*keys.Registry, error) {
	var all []keys.Location
	all = append(all, session.ValueLocations()...)
	all = append(all, sources.ValueLocations()...)
	all = append(all, instancesettings.ValueLocations()...)
	all = append(all, secrets.ValueLocations()...)
	return keys.NewRegistry(all...)
}

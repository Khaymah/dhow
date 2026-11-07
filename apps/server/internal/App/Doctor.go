package app

import (
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	installer "github.com/Khaymah/dhow/apps/server/internal/Core/Installer"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	system "github.com/Khaymah/dhow/apps/server/internal/Modules/System"
)

func doctorFor(in Infra) system.DoctorOptions {
	dir := in.Config.Artifacts.BlobDir
	if dir == "" {
		dir = config.DefaultArtifacts().BlobDir
	}
	return system.DoctorOptions{Host: installer.NewHost(), StateDir: dir, Clock: sqlc.New(in.Pool), Now: in.Now}
}

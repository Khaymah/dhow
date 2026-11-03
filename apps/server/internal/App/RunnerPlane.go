package app

import (
	"net/http"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	middleware "github.com/Khaymah/dhow/apps/server/internal/Core/Middleware"
	artifacts "github.com/Khaymah/dhow/apps/server/internal/Modules/Artifacts"
	jobs "github.com/Khaymah/dhow/apps/server/internal/Modules/Jobs"
	runners "github.com/Khaymah/dhow/apps/server/internal/Modules/Runners"
)

func RunnerPlane(o Options) http.Handler {
	mux := http.NewServeMux()
	runners.ConnectRoutes(mux, runners.Deps{Service: o.Runners, Gate: o.Gate, Log: o.Log})
	if o.Jobs != nil {
		jobs.ConnectRoutes(mux, jobs.Deps{Service: o.Jobs, Gate: o.Gate, Log: o.Log})
	}
	if o.Artifacts != nil {
		artifacts.ConnectRoutes(mux, artifacts.Deps{Service: o.Artifacts, Gate: o.Gate, Log: o.Log})
	}
	h := http.StripPrefix(config.RunnerAPIPrefix, mux)
	stack := []func(http.Handler) http.Handler{
		middleware.RequestID,
		middleware.ClientIP(o.Config.TrustedProxies),
		middleware.AccessLog(o.Log),
		middleware.Recover(o.Log),
		middleware.NoStore,
		runnerBodyLimit,
	}
	for i := len(stack) - 1; i >= 0; i-- {
		h = stack[i](h)
	}
	return h
}

func RunnerPlaneServer(o Options) http.Handler {
	root := http.NewServeMux()
	root.Handle(config.RunnerAPIPrefix+"/", RunnerPlane(o))
	return root
}

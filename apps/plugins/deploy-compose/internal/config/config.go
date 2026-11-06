package config

import (
	"fmt"
	"os"
	"time"

	"github.com/Khaymah/dhow/apps/plugins/deploy-compose/internal/policy"
	"github.com/Khaymah/dhow/apps/plugins/deploy-compose/internal/stack"
	"github.com/Khaymah/dhow/internal/deployplugin"
	"github.com/Khaymah/dhow/internal/jobspec"
)

const (
	minWaitTimeout = 5 * time.Second
	minPullTimeout = 10 * time.Second
	maxPullTimeout = 30 * time.Minute
)

type Settings struct {
	deployplugin.Common
	Project     string
	File        string
	AllowUnsafe bool
	Images      map[string]string
	WaitTimeout time.Duration
	PullTimeout time.Duration
}

func Load() (Settings, error) { return Parse(os.Getenv) }

func Parse(getenv func(string) string) (Settings, error) {
	common, err := deployplugin.Parse(getenv, jobspec.TargetKindCompose)
	if err != nil {
		return Settings{}, err
	}
	t := common.Step.Settings
	switch {
	case !jobspec.ValidComposeProject(t.ComposeProject):
		return Settings{}, fmt.Errorf("%w: the compose project name", deployplugin.ErrInput)
	case !jobspec.ValidComposeFile(t.ComposeFile) || t.ComposeFile == stack.OverrideFile:
		return Settings{}, fmt.Errorf("%w: the compose file", deployplugin.ErrInput)
	case len(t.Shared) > 0 || t.ReloadCommand != "" || t.PreSwitchCommand != "" || t.PostSwitchCommand != "":
		return Settings{}, fmt.Errorf("%w: a compose target has no hooks and no shared paths", deployplugin.ErrInput)
	}
	images, err := policy.ParseImages(common.Input("images"))
	if err != nil {
		return Settings{}, fmt.Errorf("%w: %w", deployplugin.ErrInput, err)
	}
	wait, err := duration(common.Input("wait_timeout"), stack.DefaultWaitTimeout, minWaitTimeout, stack.MaxWaitTimeout, "wait_timeout")
	if err != nil {
		return Settings{}, err
	}
	pull, err := duration(common.Input("pull_timeout"), stack.DefaultPullTimeout, minPullTimeout, maxPullTimeout, "pull_timeout")
	if err != nil {
		return Settings{}, err
	}
	return Settings{
		Common: common, Project: t.ComposeProject, File: t.ComposeFile, AllowUnsafe: t.AllowUnsafe, Images: images,
		WaitTimeout: wait, PullTimeout: pull,
	}, nil
}

func duration(v string, def, lo, hi time.Duration, name string) (time.Duration, error) {
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < lo || d > hi {
		return 0, fmt.Errorf("%w: %s must be between %s and %s", deployplugin.ErrInput, name, lo, hi)
	}
	return d, nil
}

package jobspec

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var ErrTargetStep = errors.New("the deploy target settings handed to the plugin are not valid")

const maxTargetEnvBytes = 64 << 10

type TargetStep struct {
	Name         string
	Kind         string
	User         string
	DeployPath   string
	ReleaseID    string
	KeepReleases int
	Hosts        []TargetHost
	Settings     TargetSettings
	KeyFile      string
}

func ValidReleaseID(id string) bool { return releaseIDPattern.MatchString(id) }

func ValidSharedName(n string) bool {
	return sharedNamePattern.MatchString(n) && n != "." && !strings.Contains(n, "..")
}

func ValidTargetUser(u string) bool { return targetUserPattern.MatchString(u) }

func ValidHook(v string) bool { return oneLine(v, MaxTargetHook) }

func decodeStrict(raw string, into any) error {
	if len(raw) == 0 || len(raw) > maxTargetEnvBytes {
		return ErrTargetStep
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil || dec.More() {
		return ErrTargetStep
	}
	return nil
}

func ParseTargetStep(getenv func(string) string) (TargetStep, error) {
	t := TargetStep{
		Name: getenv(EnvTargetName), Kind: getenv(EnvTargetKind), User: getenv(EnvTargetUser), DeployPath: getenv(EnvTargetDeployPath),
		ReleaseID: getenv(EnvTargetReleaseID), KeyFile: getenv(EnvTargetKeyFile),
	}
	keep, err := strconv.Atoi(getenv(EnvTargetKeepRelease))
	if err != nil {
		return TargetStep{}, fmt.Errorf("%w: %s", ErrTargetStep, EnvTargetKeepRelease)
	}
	t.KeepReleases = keep
	if err := decodeStrict(getenv(EnvTargetHosts), &t.Hosts); err != nil {
		return TargetStep{}, fmt.Errorf("%w: %s", ErrTargetStep, EnvTargetHosts)
	}
	if err := decodeStrict(getenv(EnvTargetSettings), &t.Settings); err != nil {
		return TargetStep{}, fmt.Errorf("%w: %s", ErrTargetStep, EnvTargetSettings)
	}
	if field := t.invalidField(); field != "" {
		return TargetStep{}, fmt.Errorf("%w: %s", ErrTargetStep, field)
	}
	return t, nil
}

func (t TargetStep) invalidField() string {
	switch {
	case !targetNamePattern.MatchString(t.Name):
		return EnvTargetName
	case t.Kind != TargetKindSSH && t.Kind != TargetKindCompose:
		return EnvTargetKind
	case !ValidTargetUser(t.User):
		return EnvTargetUser
	case !ValidDeployPath(t.DeployPath):
		return EnvTargetDeployPath
	case !ValidReleaseID(t.ReleaseID):
		return EnvTargetReleaseID
	case t.KeepReleases < MinKeepReleases || t.KeepReleases > MaxKeepReleases:
		return EnvTargetKeepRelease
	case !strings.HasPrefix(t.KeyFile, "/"):
		return EnvTargetKeyFile
	case len(t.Hosts) < 1 || len(t.Hosts) > MaxTargetHosts:
		return EnvTargetHosts
	}
	for _, h := range t.Hosts {
		if !validID(h.HostID) || !validTargetHost(h.Host) || h.Port < 1 || h.Port > maxPort || !validKnownHostsLine(h.Host, h.Port, h.KnownHostsLine) {
			return EnvTargetHosts
		}
	}
	return t.invalidSettings()
}

func (t TargetStep) invalidSettings() string {
	s := t.Settings
	for _, h := range []string{s.ReloadCommand, s.PreSwitchCommand, s.PostSwitchCommand} {
		if !ValidHook(h) {
			return EnvTargetSettings
		}
	}
	if len(s.Shared) > MaxTargetShared {
		return EnvTargetSettings
	}
	for _, n := range s.Shared {
		if !ValidSharedName(n) {
			return EnvTargetSettings
		}
	}
	return ""
}

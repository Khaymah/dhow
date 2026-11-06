package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/apps/plugins/deploy-ssh/internal/config"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

func TestTheRollbackModeNamesTheReleaseOnTheServerAndKeepsTheReleaseOfTheReport(t *testing.T) {
	hosts := []*runnerv1.TargetHost{host("web-1.example.com")}
	s, err := config.Parse(env(t, hosts, map[string]string{"DHOW_INPUT_MODE": "rollback", "DHOW_INPUT_RELEASE": "7-aaaaaaaa-1"}))
	require.NoError(t, err)
	assert.Equal(t, "rollback", s.Mode)
	assert.True(t, s.Deploy.Rollback)
	assert.Equal(t, "7-aaaaaaaa-1", s.Deploy.ReleaseID, "the directory on the server is the earlier release")
	assert.Equal(t, "12-abcdef12-1", s.ReportID, "the report is under the release of this run")

	s, err = config.Parse(env(t, hosts, map[string]string{
		"DHOW_INPUT_MODE": "rollback", "DHOW_INPUT_RELEASE": "7-aaaaaaaa-1", "DHOW_INPUT_ARTIFACT_FILE": "", "DHOW_INPUT_ARTIFACT_DIGEST": "",
	}))
	require.NoError(t, err, "a rollback needs no artifact when every server still has the release")
	assert.Empty(t, s.Deploy.ArchivePath)
}

func TestTheRollbackModeRefusesAMissingOrOwnReleaseAndADeployStillNeedsItsArtifact(t *testing.T) {
	hosts := []*runnerv1.TargetHost{host("web-1.example.com")}
	for name, extra := range map[string]map[string]string{
		"no release":      {"DHOW_INPUT_MODE": "rollback"},
		"a bad release":   {"DHOW_INPUT_MODE": "rollback", "DHOW_INPUT_RELEASE": "../x"},
		"its own release": {"DHOW_INPUT_MODE": "rollback", "DHOW_INPUT_RELEASE": "12-abcdef12-1"},
		"a bad digest":    {"DHOW_INPUT_MODE": "rollback", "DHOW_INPUT_RELEASE": "7-aaaaaaaa-1", "DHOW_INPUT_ARTIFACT_DIGEST": "xyz"},
		"deploy no file":  {"DHOW_INPUT_ARTIFACT_FILE": ""},
	} {
		_, err := config.Parse(env(t, hosts, extra))
		require.ErrorIs(t, err, config.ErrInput, name)
	}
}

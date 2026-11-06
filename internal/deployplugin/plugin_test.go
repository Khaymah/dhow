package deployplugin_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/deployplugin"
	"github.com/Khaymah/dhow/internal/deployrelease"
)

func TestEveryExitOfTheEngineHasANamedCode(t *testing.T) {
	for exit, code := range map[int]string{
		deployrelease.ExitInput:           "PLUGIN_INPUT_INVALID",
		deployrelease.ExitRolledBack:      "DEPLOY_ROLLED_BACK",
		deployrelease.ExitHealthFailed:    "DEPLOY_HEALTH_FAILED",
		deployrelease.ExitRollbackFailed:  "DEPLOY_ROLLBACK_FAILED",
		deployrelease.ExitHostKeyMismatch: "HOST_KEY_MISMATCH",
		deployrelease.ExitUnsafeArtifact:  "ARTIFACT_UNSAFE",
		deployrelease.ExitTargetLocked:    "TARGET_LOCKED",
		deployrelease.ExitCurrentNotLink:  "CURRENT_NOT_A_SYMLINK",
		deployrelease.ExitConnect:         "TARGET_HOST_UNREACHABLE",
		deployrelease.ExitFailed:          "DEPLOY_FAILED",
		99:                                "DEPLOY_FAILED",
	} {
		assert.Equal(t, code, deployplugin.Code(exit))
	}
}

func TestTheKeyFileMustBeThereAndSmall(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good")
	require.NoError(t, os.WriteFile(good, []byte("key"), 0o400))
	raw, err := deployplugin.ReadKey(good)
	require.NoError(t, err)
	assert.Equal(t, "key", string(raw))

	empty := filepath.Join(dir, "empty")
	require.NoError(t, os.WriteFile(empty, nil, 0o400))
	big := filepath.Join(dir, "big")
	require.NoError(t, os.WriteFile(big, []byte(strings.Repeat("a", 16<<10+1)), 0o400))
	for _, path := range []string{empty, big, filepath.Join(dir, "missing")} {
		_, err := deployplugin.ReadKey(path)
		require.Error(t, err, path)
	}
}

func TestAMissingKeyStopsTheRunBeforeAnyConnection(t *testing.T) {
	c := deployplugin.Common{KeyFile: filepath.Join(t.TempDir(), "missing")}
	f := deployplugin.Run(t.Context(), c, nilLogger())
	require.NotNil(t, f)
	assert.Equal(t, deployrelease.ExitInput, f.Exit)
}

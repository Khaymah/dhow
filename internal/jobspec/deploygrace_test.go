package jobspec_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/Khaymah/dhow/internal/jobspec"
)

func TestTheCancelGraceOfADeployStepIsBuiltFromTheBudgetsOfTheRollback(t *testing.T) {
	want := jobspec.DeployRollbackBudget + jobspec.DeployConnGrace + jobspec.DeployCleanupBudget + jobspec.DeployUnlockBudget
	assert.Equal(t, want, jobspec.DeployCancelGrace())
	assert.Equal(t, 3*time.Minute+55*time.Second, jobspec.DeployCancelGrace())
	assert.Greater(t, jobspec.DeployCancelGrace(), jobspec.DeployRollbackBudget, "one switch, one reload and the rest of the rollback fit in the grace")
}

func TestOnlyTheOfficialDeployImagesAreOfficial(t *testing.T) {
	digest := "@sha256:" + strings.Repeat("a", 64)
	for image, want := range map[string]bool{
		"ghcr.io/khaymah/dhow-deploy-ssh" + digest:          true,
		"ghcr.io/khaymah/dhow-deploy-compose" + digest:      true,
		"ghcr.io/khaymah/dhow-deploy-ssh:1.2.3":             true,
		"ghcr.io/khaymah/dhow-deploy-ssh":                   true,
		"ghcr.io/khaymah/dhow-deploy-ssh-evil" + digest:     false,
		"ghcr.io/khaymah/dhow-deploy-ssh/evil" + digest:     false,
		"ghcr.io/acme/dhow-deploy-ssh" + digest:             false,
		"evil.example.com/khaymah/dhow-deploy-ssh" + digest: false,
		"":                                     false,
		"ghcr.io/khaymah/dhow-deploy-sshx:tag": false,
		"ghcr.io/khaymah/dhow-deploy-ssh@sha256:not-a-digest": true,
	} {
		assert.Equal(t, want, jobspec.IsOfficialDeployImage(image), image)
	}
}

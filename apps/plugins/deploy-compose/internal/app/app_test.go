package app_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Khaymah/dhow/apps/plugins/deploy-compose/internal/app"
	"github.com/Khaymah/dhow/apps/plugins/deploy-compose/internal/policy"
	"github.com/Khaymah/dhow/apps/plugins/deploy-compose/internal/stack"
	"github.com/Khaymah/dhow/internal/deployrelease"
)

func TestThePrintedCodeNamesTheRuleThatRefusedTheFile(t *testing.T) {
	refused := &deployrelease.Failure{Exit: stack.ExitPolicy, Text: "refused", Err: fmt.Errorf("wrapped: %w", &policy.Violation{Code: "COMPOSE_UNSAFE_PRIVILEGED", Service: "web"})}
	assert.Equal(t, "COMPOSE_UNSAFE_PRIVILEGED", app.Code(refused))
	assert.Equal(t, "COMPOSE_INVALID", app.Code(&deployrelease.Failure{Exit: stack.ExitPolicy}))
}

func TestTheOtherExitsKeepTheCodesOfTheSshPlugin(t *testing.T) {
	assert.Equal(t, "DEPLOY_ROLLED_BACK", app.Code(&deployrelease.Failure{Exit: deployrelease.ExitRolledBack}))
	assert.Equal(t, "HOST_KEY_MISMATCH", app.Code(&deployrelease.Failure{Exit: deployrelease.ExitHostKeyMismatch}))
	assert.Equal(t, "DEPLOY_FAILED", app.Code(&deployrelease.Failure{Exit: 99}))
}

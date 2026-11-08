package unit_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
)

func TestEveryMemberRoleKeepsProjectReadWithoutAnyBinding(t *testing.T) {
	for _, role := range policy.Roles() {
		assert.True(t, policy.Allows(policy.ProjectRole(role, nil), policy.ProjectRead), string(role))
	}
}

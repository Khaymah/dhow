package policy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNotificationManageIsAProjectActionForMaintainersAndAbove(t *testing.T) {
	assert.True(t, IsProjectAction(NotificationManage))
	assert.True(t, IsSensitive(NotificationManage))
	assert.False(t, IsTokenAction(NotificationManage), "an API token never manages a notification channel")
	assert.Contains(t, Actions(), NotificationManage)
	for role, want := range map[Role]bool{
		RoleOwner: true, RoleAdmin: true, RoleMaintainer: true, RoleDeveloper: false, RoleViewer: false,
	} {
		assert.Equal(t, want, Allows(role, NotificationManage), string(role))
	}
}

func TestOrgNotificationManageIsAnOrganizationActionForOwnersAndAdmins(t *testing.T) {
	assert.False(t, IsProjectAction(OrgNotificationManage))
	assert.True(t, IsSensitive(OrgNotificationManage))
	assert.False(t, IsTokenAction(OrgNotificationManage))
	assert.Contains(t, Actions(), OrgNotificationManage)
	for role, want := range map[Role]bool{
		RoleOwner: true, RoleAdmin: true, RoleMaintainer: false, RoleDeveloper: false, RoleViewer: false,
	} {
		assert.Equal(t, want, Allows(role, OrgNotificationManage), string(role))
	}
}

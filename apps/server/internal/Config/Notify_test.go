package config_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
)

func TestASendOverTheMailServerEndsBeforeTheLeaseOfItsDeliveryDoes(t *testing.T) {
	assert.Less(t, config.MailTimeout, config.NotifyClaimLease, "else a second worker could claim a delivery that is still being sent")
	assert.Less(t, config.NotifyRequestTimeout, config.NotifyClaimLease)
}

func TestTheSharedPoolKeepsRoomForEveryOrganizationAndForAccountMail(t *testing.T) {
	assert.Less(t, config.NotifyOrgConcurrency, config.NotifyGlobalConcurrency)
	assert.Positive(t, config.NotifyMailSlots)
}

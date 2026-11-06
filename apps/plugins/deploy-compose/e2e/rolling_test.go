package e2e_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/deployrelease"
)

func TestTheSecondServerFailingInTheSecondBatchRollsTheFirstBack(t *testing.T) {
	d := newDeployment(t, "fleet", 8090, 2)
	one, two := d.hosts[0], d.hosts[1]
	v1, v2 := publishApp(t, "v1"), publishApp(t, "v2")
	one.setBad(t, v1, "")
	two.setBad(t, v1, "")

	first := d.deploy("", spec{release: releaseOne, compose: d.compose(v1, stateCheck(), ""), batch: 1, health: true})
	require.Equal(t, 0, first.exit, first.output)
	for _, h := range d.hosts {
		assert.Equal(t, "v1", h.served(d.port), h.name)
		assert.Equal(t, []string{v1}, h.runningImages(d.project), h.name)
	}

	two.setBad(t, v1, "v2")
	res := d.deploy("rolling-compose-second-fails", spec{release: releaseTwo, compose: d.compose(v2, stateCheck(), ""), batch: 1, health: true})
	assert.Equal(t, deployrelease.ExitRolledBack, res.exit, res.output)
	group, ok := res.groupEvent(deployrelease.EventRolledBack)
	require.True(t, ok, "the group is rolled_back")
	assert.Equal(t, deployrelease.CauseReload, group.Reason)
	assert.True(t, res.has(deployrelease.EventBatchPassed), "the first batch passed before the second failed")
	assert.False(t, res.has(deployrelease.EventVerified))

	for _, h := range d.hosts {
		assert.Equal(t, "releases/"+releaseOne, h.current(d.path), h.name)
		assert.Equal(t, []string{v1}, h.runningImages(d.project), "%s runs the images of its previous release", h.name)
	}
	assert.Equal(t, "v1", one.served(d.port))
	rolled := map[string]bool{}
	for _, e := range res.events {
		if e.Event == deployrelease.EventRolledBack && e.HostID != "" {
			rolled[e.HostID] = true
		}
	}
	assert.Len(t, rolled, 2, "both servers were switched, so both are rolled back")
}

func TestAMissingImageOnTheSecondServerStopsBeforeAnySwitch(t *testing.T) {
	d := newDeployment(t, "fleetpull", 8091, 2)
	v1 := publishApp(t, "v1")
	for _, h := range d.hosts {
		h.setBad(t, v1, "")
	}
	require.Equal(t, 0, d.deploy("", spec{release: releaseOne, compose: d.compose(v1, stateCheck(), ""), batch: 1}).exit)

	gone := "localhost:5000/app@sha256:" + "1111111111111111111111111111111111111111111111111111111111111111"
	res := d.deploy("rolling-compose-missing-image", spec{release: releaseTwo, compose: d.compose(gone, stateCheck(), ""), batch: 1})
	assert.Equal(t, deployrelease.ExitFailed, res.exit, res.output)
	assert.False(t, res.has(deployrelease.EventSwitched))
	for _, h := range d.hosts {
		assert.Equal(t, "releases/"+releaseOne, h.current(d.path), h.name)
		assert.Equal(t, "v1", h.served(d.port), h.name)
	}
}

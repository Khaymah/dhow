package config

import "time"

const (
	OrgEventsChannel    = "dhow_org_events"
	OrgEventsRing       = 4096
	OrgEventsBatch      = 256
	OrgEventsTick       = 15 * time.Second
	OrgEventsWriteWait  = 10 * time.Second
	OrgEventsRetry      = 3 * time.Second
	OrgEventsRetryAfter = 5 * time.Second
	OrgEventsPerUser    = 5
	OrgEventsOrgShare   = 10
	OrgEventsOpenBurst  = 20
	OrgEventsOpenRefill = time.Second
	OrgEventsListenMin  = time.Second
	OrgEventsListenMax  = 30 * time.Second
	OrgEventsTracked    = 10_000
	OrgEventsMaxPayload = 7000
)

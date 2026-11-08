package config

import (
	"errors"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/Khaymah/dhow/internal/netguard"
)

const (
	EnvNotifyAllowPrivateCIDRs = "DHOW_NOTIFY_ALLOW_PRIVATE_CIDRS"
	EnvNotifyRetentionDays     = "DHOW_NOTIFY_RETENTION_DAYS"

	NotifyRetentionDefault    = 30
	NotifyRetentionMin        = 1
	NotifyRetentionMax        = 365
	NotifyMaxAttempts         = 5
	NotifyChannelConcurrency  = 2
	NotifyGlobalConcurrency   = 16
	NotifyConnectTimeout      = 5 * time.Second
	NotifyRequestTimeout      = 10 * time.Second
	NotifyResponseReadMax     = 64 << 10
	NotifyBreakerFailures     = 10
	NotifyClaimLease          = 2 * time.Minute
	NotifyPollInterval        = 2 * time.Second
	NotifyLockKey             = int64(0x64686f772d6e7479)
	NotifyTestPerMinute       = 5
	NotifyTestWindow          = time.Minute
	NotifySecretBytes         = 32
	NotifyChannelNameMax      = 64
	NotifyChannelsPerScope    = 20
	NotifyRolesMax            = 5
	NotifyOfflineAfter        = 5 * time.Minute
	NotifyOfflineScanEvery    = time.Minute
	NotifyURLMax              = 2048
	NotifyConfigSealLabel     = "dhow/notification-channel/v1"
	NotifyDeliveriesMax       = 100
	NotifyDeviceMemory        = 50
	NotifyUserAgentMax        = 512
	NotifyOrgConcurrency      = 4
	NotifyMailSlots           = 2
	NotifyShutdownWait        = 5 * time.Second
	NotifyRecipientsPerEvent  = 100
	NotifyEmailPerUserHour    = 30
	NotifyEmailWindow         = time.Hour
	NotifyTestPerUser         = 10
	NotifyTestPerOrg          = 20
	NotifyInboxPerUser        = 200
	NotifyInboxAccountPerUser = 50
	NotifyInboxPurgeBatch     = 1000
	NotifyInboxLinkMax        = 512
	NotifyInboxPurgeEvery     = time.Hour
)

func NotifyRetrySchedule() []time.Duration {
	return []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute, 2 * time.Hour, 6 * time.Hour}
}

type Notify struct {
	AllowPrivate  []netip.Prefix
	RetentionDays int
}

func DefaultNotify() Notify { return Notify{RetentionDays: NotifyRetentionDefault} }

func loadNotify(getenv func(string) string) (Notify, []error) {
	out := DefaultNotify()
	var errs []error
	allowed, err := netguard.ParseCIDRs(getenv(EnvNotifyAllowPrivateCIDRs))
	if err != nil {
		errs = append(errs, errors.New(EnvNotifyAllowPrivateCIDRs+": "+err.Error()))
	} else {
		out.AllowPrivate = allowed
	}
	if v := strings.TrimSpace(getenv(EnvNotifyRetentionDays)); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < NotifyRetentionMin || n > NotifyRetentionMax {
			errs = append(errs, errors.New(EnvNotifyRetentionDays+": must be a whole number of days from "+
				strconv.Itoa(NotifyRetentionMin)+" to "+strconv.Itoa(NotifyRetentionMax)))
		} else {
			out.RetentionDays = n
		}
	}
	return out, errs
}

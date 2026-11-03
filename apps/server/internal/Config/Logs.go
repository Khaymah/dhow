package config

import (
	"errors"
	"strconv"
	"strings"
	"time"

	settingspec "github.com/Khaymah/dhow/apps/server/internal/Config/Settings"
)

const (
	EnvJobLogMaxBytes    = settingspec.EnvJobLogMaxBytes
	EnvLogRetentionDays  = settingspec.EnvLogRetentionDays
	EnvSSEMaxStreams     = "DHOW_SSE_MAX_STREAMS"
	EnvLogReadConns      = "DHOW_LOG_READ_CONNS"
	LogAppendedChannel   = "dhow_log_appended"
	LogChunkMaxBytes     = 64 << 10
	LogAppendMaxBytes    = RunnerBodyMaxBytes
	LogAppendMaxChunks   = 1024
	LogMaxStepIndex      = 99
	LogPageMaxBytes      = 1 << 20
	LogPageDefaultLimit  = 100
	LogPageMaxLimit      = 500
	LogStreamBatchRows   = 64
	LogStreamBatchBytes  = 256 << 10
	LogStreamTick        = 15 * time.Second
	LogStreamWriteWait   = 10 * time.Second
	LogStreamRetry       = 3 * time.Second
	LogStreamsPerUser    = 10
	LogStreamRetryAfter  = 5 * time.Second
	LogPartitionsAhead   = 3
	LogMaintainInterval  = time.Hour
	LogMaintainLockWait  = 2 * time.Second
	LogMaxBytesDefault   = settingspec.JobLogMaxBytesDefault
	LogMaxBytesMin       = settingspec.JobLogMaxBytesMin
	LogMaxBytesMax       = settingspec.JobLogMaxBytesMax
	LogRetentionDefault  = settingspec.LogRetentionDefault
	LogRetentionMax      = settingspec.LogRetentionMax
	OrgLogRetentionMin   = 7
	SSEMaxStreamsDefault = 2000
	SSEMaxStreamsMax     = 100_000
	LogReadConnsDefault  = 4
	LogReadStatementWait = 5 * time.Second
	LogReadWait          = 10 * time.Second
	LogOrgStreamsShare   = 10
	LogReadBurst         = 30
	LogReadRefill        = 200 * time.Millisecond
	LogStreamOpenBurst   = 20
	LogStreamOpenRefill  = time.Second
	LogTrackedUsers      = 10_000
)

type LogSettings = settingspec.Logs

func DefaultLogSettings() LogSettings {
	return LogSettings{RetentionDays: LogRetentionDefault, JobLogMaxBytes: LogMaxBytesDefault}
}

type Logs struct {
	MaxStreams int
	ReadConns  int
}

func DefaultLogs() Logs {
	return Logs{MaxStreams: SSEMaxStreamsDefault, ReadConns: LogReadConnsDefault}
}

func (l Logs) OrgStreams() int { return max(2*LogStreamsPerUser, l.MaxStreams/LogOrgStreamsShare) }

func boundedSmall(v string, def, lowest, highest int) (int, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return def, nil
	}
	n, err := strconv.ParseInt(v, 10, 32)
	if err != nil || n < int64(lowest) || n > int64(highest) {
		return 0, errors.New("must be a whole number from " + strconv.Itoa(lowest) + " to " + strconv.Itoa(highest))
	}
	return int(n), nil
}

func loadLogs(getenv func(string) string) (Logs, []error) {
	out := DefaultLogs()
	var errs []error
	streams, err := boundedSmall(getenv(EnvSSEMaxStreams), SSEMaxStreamsDefault, 1, SSEMaxStreamsMax)
	if err != nil {
		errs = append(errs, errors.New(EnvSSEMaxStreams+": "+err.Error()))
	}
	conns, err := boundedSmall(getenv(EnvLogReadConns), LogReadConnsDefault, 1, DatabaseMaxConns)
	if err != nil {
		errs = append(errs, errors.New(EnvLogReadConns+": "+err.Error()))
	}
	if len(errs) > 0 {
		return out, errs
	}
	return Logs{MaxStreams: streams, ReadConns: conns}, nil
}

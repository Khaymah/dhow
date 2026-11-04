package jobspec

import "time"

const (
	MaxGrantBytes      = 256 << 10
	GrantOverheadBytes = 4 << 10
	MaxSpecBytes       = MaxGrantBytes - GrantOverheadBytes

	MaxImageLength   = 512
	DefaultRegistry  = "docker.io"
	DefaultNamespace = "library"

	MaxSteps          = 100
	MaxStepNameLength = 100
	MaxRunBytes       = 64 << 10

	MaxEnvEntries     = 100
	MaxEnvNameLength  = 128
	MaxEnvValueBytes  = 32 << 10
	ReservedEnvPrefix = "DHOW_"

	MaxSecrets            = 50
	MaxSecretNameLength   = 64
	MaxSecretFileLength   = 128
	MaxSecretFiles        = 20
	MaxEgressEntries      = 64
	MaxEgressEntryLength  = 260
	MaxLabels             = 32
	MaxLabelLength        = 128
	MinTimeoutSeconds     = 1
	MaxTimeoutSeconds     = int32(24 * time.Hour / time.Second)
	DefaultTimeoutSeconds = int32(time.Hour / time.Second)

	MinCPUMillis     = 100
	MaxCPUMillis     = 64_000
	DefaultCPUMillis = 1_000

	MinMemoryBytes     = 64 << 20
	MaxMemoryBytes     = 64 << 30
	DefaultMemoryBytes = 2 << 30

	MinPids     = 32
	MaxPids     = 32_768
	DefaultPids = 1_024

	MinDiskBytes     = 64 << 20
	MaxDiskBytes     = 100 << 30
	DefaultDiskBytes = 4 << 30
)

const (
	ReasonRequired   = "required"
	ReasonInvalid    = "invalid"
	ReasonTooLong    = "too_long"
	ReasonTooMany    = "too_many"
	ReasonOutOfRange = "out_of_range"
	ReasonReserved   = "reserved"
	ReasonDuplicate  = "duplicate"
	ReasonNotPinned  = "not_pinned"
)

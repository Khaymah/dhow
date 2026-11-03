package sandbox

const (
	PackCommand       = "__artifact-pack"
	UnpackCommand     = "__artifact-unpack"
	HashFilesCommand  = "__hash-files"
	TransferStepIndex = int32(-2)
	UnpackMaxEnv      = "DHOW_UNPACK_MAX_BYTES"

	TransferExitOK          = 0
	TransferExitFailed      = 1
	TransferExitMissing     = 3
	TransferExitUnsafe      = 4
	TransferExitTooLarge    = 5
	TransferExitUnsupported = 6
)

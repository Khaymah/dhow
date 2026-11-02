package pipelinespec

import "time"

const (
	MaxFileBytes   = 256 << 10
	MaxScalarBytes = 64 << 10
	MaxDepth       = 32
	MaxNodes       = 50_000
	MaxIssues      = 200

	SupportedVersion = 1
	MaxNameLength    = 100

	MaxJobs            = 64
	MaxRunJobs         = 256
	MaxMatrixJobs      = 256
	MaxMatrixRaw       = 4096
	MaxMatrixAxes      = 16
	MaxMatrixValue     = 128
	MaxMatrixNameRunes = 512
	MaxNeeds           = 64

	MaxPatterns      = 50
	MaxPatternLength = 256

	MaxManualInputs  = 20
	MaxChoiceOptions = 50
	MaxDescription   = 500

	MaxPluginInputs     = 32
	MaxPluginInputBytes = 32 << 10

	MaxExpressionBytes     = 2 << 10
	MaxExpressionCost      = 10_000
	MaxExpressions         = 1024
	MaxTotalExpressionCost = 1_000_000
	MaxExpressionNodes     = 512
	MaxExpressionDepth     = 32
	ExpressionTimeout      = 10 * time.Millisecond
	InterruptCheckEvery    = 64

	MaxMessageBytes      = 64 << 10
	MaxContextValueBytes = 32 << 10
	MaxContextVarBytes   = 4 << 10
	MaxContextTextBytes  = 1 << 10
	MaxContextVars       = 256
	MaxContextEnv        = 100
	MaxContextOutputs    = 64

	MaxArtifacts       = 16
	MaxPaths           = 32
	MaxPathLength      = 512
	MaxArtifactName    = 64
	MaxEnvironmentName = 64
	MaxRetention       = 30 * 24 * time.Hour
	MinRetention       = 24 * time.Hour
	MaxDurationDigits  = 6
	MaxKeyBytes        = 256
	MaxPathBytes       = 512
	hashHexLength      = 64
)

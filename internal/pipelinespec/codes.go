package pipelinespec

const (
	CodeFileTooLarge           = "FILE_TOO_LARGE"
	CodeFileEmpty              = "FILE_EMPTY"
	CodeYAMLSyntax             = "YAML_SYNTAX"
	CodeMultipleDocuments      = "MULTIPLE_DOCUMENTS"
	CodeDepthExceeded          = "DEPTH_EXCEEDED"
	CodeTooManyNodes           = "TOO_MANY_NODES"
	CodeScalarTooLarge         = "SCALAR_TOO_LARGE"
	CodeAliasNotAllowed        = "ALIAS_NOT_ALLOWED"
	CodeAnchorNotAllowed       = "ANCHOR_NOT_ALLOWED"
	CodeMergeKeyNotAllowed     = "MERGE_KEY_NOT_ALLOWED"
	CodeTagNotAllowed          = "TAG_NOT_ALLOWED"
	CodeDuplicateKey           = "DUPLICATE_KEY"
	CodeKeyInvalid             = "KEY_INVALID"
	CodeUnknownKey             = "UNKNOWN_KEY"
	CodeTypeMismatch           = "TYPE_MISMATCH"
	CodeTriggerNotSupported    = "TRIGGER_NOT_SUPPORTED_YET"
	CodeVersionUnsupported     = "VERSION_UNSUPPORTED"
	CodeNameInvalid            = "NAME_INVALID"
	CodeTriggersRequired       = "TRIGGERS_REQUIRED"
	CodePatternInvalid         = "PATTERN_INVALID"
	CodeTooManyPatterns        = "TOO_MANY_PATTERNS"
	CodeInputInvalid           = "INPUT_INVALID"
	CodeTooManyInputs          = "TOO_MANY_INPUTS"
	CodeJobsRequired           = "JOBS_REQUIRED"
	CodeTooManyJobs            = "TOO_MANY_JOBS"
	CodeJobIDInvalid           = "JOB_ID_INVALID"
	CodeNeedUnknown            = "NEED_UNKNOWN"
	CodeNeedDuplicate          = "NEED_DUPLICATE"
	CodeDAGCycle               = "DAG_CYCLE"
	CodeStepsRequired          = "STEPS_REQUIRED"
	CodeTooManySteps           = "TOO_MANY_STEPS"
	CodeStepRunOrUses          = "STEP_RUN_OR_USES"
	CodeStepIDInvalid          = "STEP_ID_INVALID"
	CodeStepIDDuplicate        = "STEP_ID_DUPLICATE"
	CodeRunExpression          = "RUN_EXPRESSION_NOT_ALLOWED"
	CodeRunInvalid             = "RUN_INVALID"
	CodeUsesNotPinned          = "USES_NOT_PINNED"
	CodeUsesInvalid            = "USES_INVALID"
	CodeWithWithoutUses        = "WITH_WITHOUT_USES"
	CodeImageRequired          = "IMAGE_REQUIRED"
	CodeImageInvalid           = "IMAGE_INVALID"
	CodeImageNotPinned         = "IMAGE_NOT_PINNED"
	CodeEnvNameInvalid         = "ENV_NAME_INVALID"
	CodeEnvNameReserved        = "ENV_NAME_RESERVED"
	CodeTooManyEnv             = "TOO_MANY_ENV"
	CodeValueTooLong           = "VALUE_TOO_LONG"
	CodeValueInvalid           = "VALUE_INVALID"
	CodeExpressionSyntax       = "EXPRESSION_SYNTAX"
	CodeExpressionTooLong      = "EXPRESSION_TOO_LONG"
	CodeExpressionUnknownName  = "EXPRESSION_UNKNOWN_NAME"
	CodeExpressionType         = "EXPRESSION_TYPE"
	CodeExpressionNotAllowed   = "EXPRESSION_NOT_ALLOWED"
	CodeExpressionTooExpensive = "EXPRESSION_TOO_EXPENSIVE"
	CodeExpressionBudget       = "EXPRESSION_BUDGET_EXCEEDED"
	CodeKeyTooLong             = "KEY_TOO_LONG"
	CodeSecretsInIf            = "SECRETS_IN_IF"
	CodeSecretsNotAllowed      = "SECRETS_NOT_ALLOWED"
	CodeSecretNotWholeValue    = "SECRET_NOT_WHOLE_VALUE"
	CodeSecretsUnavailable     = "SECRETS_UNAVAILABLE"
	CodeEnvironmentInvalid     = "ENVIRONMENT_INVALID"
	CodeEnvironmentsDeferred   = "ENVIRONMENTS_UNAVAILABLE"
	CodeMatrixTooLarge         = "MATRIX_TOO_LARGE"
	CodeMatrixEmpty            = "MATRIX_EMPTY"
	CodeMatrixKeyInvalid       = "MATRIX_KEY_INVALID"
	CodeMatrixValueInvalid     = "MATRIX_VALUE_INVALID"
	CodeMatrixEntryInvalid     = "MATRIX_ENTRY_INVALID"
	CodeMatrixDuplicate        = "MATRIX_DUPLICATE"
	CodeMatrixNameTooLong      = "MATRIX_NAME_TOO_LONG"
	CodeMaxParallelInvalid     = "MAX_PARALLEL_INVALID"
	CodeTooManyRunJobs         = "TOO_MANY_RUN_JOBS"
	CodeTimeoutInvalid         = "TIMEOUT_INVALID"
	CodeResourceInvalid        = "RESOURCE_INVALID"
	CodeEgressInvalid          = "EGRESS_INVALID"
	CodeTooManyEgress          = "TOO_MANY_EGRESS"
	CodeLabelInvalid           = "LABEL_INVALID"
	CodeTooManyLabels          = "TOO_MANY_LABELS"
	CodeIsolationNotSupported  = "ISOLATION_NOT_SUPPORTED_YET"
	CodeIsolationInvalid       = "ISOLATION_INVALID"
	CodeArtifactNameInvalid    = "ARTIFACT_NAME_INVALID"
	CodeArtifactDuplicate      = "ARTIFACT_NAME_DUPLICATE"
	CodeTooManyArtifacts       = "TOO_MANY_ARTIFACTS"
	CodePathInvalid            = "PATH_INVALID"
	CodeTooManyPaths           = "TOO_MANY_PATHS"
	CodePathsRequired          = "PATHS_REQUIRED"
	CodeRetentionInvalid       = "RETENTION_INVALID"
	CodeCacheKeyRequired       = "CACHE_KEY_REQUIRED"
	CodeGroupRequired          = "CONCURRENCY_GROUP_REQUIRED"
	CodeSpecTooLarge           = "JOB_SPEC_TOO_LARGE"
	CodeSpecInvalid            = "JOB_SPEC_INVALID"
)

func message(code string) string {
	if m := parseMessage(code); m != "" {
		return m
	}
	if m := jobMessage(code); m != "" {
		return m
	}
	if m := valueMessage(code); m != "" {
		return m
	}
	return "the pipeline is invalid here"
}

func parseMessage(code string) string {
	switch code {
	case CodeFileTooLarge:
		return "the file is larger than 256 KiB"
	case CodeFileEmpty:
		return "the file is empty"
	case CodeYAMLSyntax:
		return "the file is not valid YAML"
	case CodeMultipleDocuments:
		return "the file holds more than one YAML document"
	case CodeDepthExceeded:
		return "the file is nested deeper than 32 levels"
	case CodeTooManyNodes:
		return "the file has more than 50,000 YAML nodes"
	case CodeScalarTooLarge:
		return "a value is larger than 64 KiB"
	case CodeAliasNotAllowed:
		return "YAML aliases (*name) are not allowed: write the value out"
	case CodeAnchorNotAllowed:
		return "YAML anchors (&name) are not allowed: write the value out"
	case CodeMergeKeyNotAllowed:
		return "YAML merge keys (<<) are not allowed: write the keys out"
	case CodeTagNotAllowed:
		return "custom YAML tags are not allowed"
	case CodeDuplicateKey:
		return "this key appears more than once in the same mapping"
	case CodeKeyInvalid:
		return "a mapping key must be a plain value"
	case CodeKeyTooLong:
		return "a mapping key is longer than 256 bytes"
	case CodeUnknownKey:
		return "this key is not part of the pipeline spec"
	case CodeTypeMismatch:
		return "this value has the wrong type"
	case CodeTriggerNotSupported:
		return "this trigger is not supported yet (schedule comes after the first release, artifact with M6)"
	}
	return ""
}

func jobMessage(code string) string {
	switch code {
	case CodeVersionUnsupported:
		return "version must be 1"
	case CodeNameInvalid:
		return "the name must be 1 to 100 characters without control characters"
	case CodeTriggersRequired:
		return "the pipeline needs at least one trigger under on"
	case CodeJobsRequired:
		return "the pipeline needs at least one job"
	case CodeTooManyJobs:
		return "a pipeline has at most 64 jobs before the matrix"
	case CodeJobIDInvalid:
		return "a job id is a lowercase letter followed by up to 62 of a-z, 0-9, _ and -"
	case CodeNeedUnknown:
		return "needs refers to a job that does not exist"
	case CodeNeedDuplicate:
		return "needs lists the same job twice"
	case CodeDAGCycle:
		return "the needs of this job form a cycle"
	case CodeStepsRequired:
		return "a job needs at least one step"
	case CodeTooManySteps:
		return "a job has at most 100 steps"
	case CodeStepRunOrUses:
		return "a step has exactly one of run or uses"
	case CodeStepIDInvalid, CodeStepIDDuplicate:
		return "a step id is unique in its job and follows the job id rules"
	case CodeRunExpression:
		return "${{ }} is not allowed inside run: pass the value through env and read it as a variable"
	case CodeRunInvalid:
		return "run must be a non-empty script of at most 64 KiB without NUL bytes"
	case CodeUsesNotPinned:
		return "a plugin in uses must be pinned by digest (name@sha256:...)"
	case CodeUsesInvalid:
		return "uses must be a container image reference"
	case CodeWithWithoutUses:
		return "with is only allowed on a uses step"
	case CodeImageRequired:
		return "a job with run steps needs an image"
	case CodeImageInvalid:
		return "image must be a container image reference"
	case CodeImageNotPinned:
		return "the image is not pinned by digest: the runner refuses it unless unpinned images are allowed"
	case CodeMatrixTooLarge:
		return "a matrix expands to at most 256 jobs"
	case CodeMatrixEmpty:
		return "the matrix has no combinations"
	case CodeMatrixKeyInvalid:
		return "a matrix key is a letter or _ followed by up to 63 letters, digits and _"
	case CodeMatrixValueInvalid:
		return "a matrix value is a string, number or boolean of at most 128 printable characters"
	case CodeMatrixEntryInvalid:
		return "an include or exclude entry must use the matrix keys"
	case CodeMatrixDuplicate:
		return "two matrix combinations give the same job name"
	case CodeMatrixNameTooLong:
		return "the matrix values of one job make its name longer than 512 characters"
	case CodeMaxParallelInvalid:
		return "max_parallel is from 1 to 256"
	case CodeTooManyRunJobs:
		return "the pipeline expands to more than 256 jobs"
	case CodeSpecTooLarge:
		return "the job is too large once expanded"
	case CodeSpecInvalid:
		return "the expanded job is not a valid job spec"
	}
	return ""
}

func valueMessage(code string) string {
	switch code {
	case CodePatternInvalid:
		return "a pattern uses only *, ** and ? as wildcards and is at most 256 characters"
	case CodeTooManyPatterns:
		return "a filter has at most 50 patterns"
	case CodeInputInvalid:
		return "the input is invalid: check its name, type, options and default"
	case CodeTooManyInputs:
		return "there are too many inputs here"
	case CodeEnvNameInvalid:
		return "an environment variable name is a letter or _ followed by letters, digits and _ (at most 128)"
	case CodeEnvNameReserved:
		return "names that start with DHOW_ are reserved"
	case CodeTooManyEnv:
		return "there are at most 100 environment variables here"
	case CodeValueTooLong:
		return "the value is too long"
	case CodeValueInvalid:
		return "the value is invalid"
	case CodeExpressionSyntax:
		return "the expression is not valid CEL, or ${{ }} is not the whole value"
	case CodeExpressionTooLong:
		return "an expression is at most 2 KiB"
	case CodeExpressionUnknownName:
		return "the expression uses a name that is not available here"
	case CodeExpressionType:
		return "the expression has the wrong type for this field"
	case CodeExpressionNotAllowed:
		return "${{ }} is not allowed in this field"
	case CodeExpressionTooExpensive:
		return "the expression could cost too much to evaluate: simplify it"
	case CodeExpressionBudget:
		return "the pipeline has more than 1,024 expressions, or they could cost too much together"
	case CodeSecretsInIf:
		return "secrets can never be used in a condition"
	case CodeSecretsNotAllowed:
		return "secrets are not allowed in this field"
	case CodeSecretNotWholeValue:
		return "a secret is the whole value (${{ secrets.NAME }}), never part of a larger expression"
	case CodeSecretsUnavailable:
		return "secrets arrive with M5: until then a job that asks for a secret fails when it is queued"
	case CodeEnvironmentInvalid:
		return "an environment name is 1 to 64 of a-z, 0-9, _ and -"
	case CodeEnvironmentsDeferred:
		return "environments arrive with M5: until then this job is skipped"
	case CodeTimeoutInvalid:
		return "a duration is like 90s, 20m or 1h30m, from 1s to 24h, and a step is not longer than its job"
	case CodeResourceInvalid:
		return "the resource value is invalid or out of range"
	case CodeEgressInvalid:
		return "an egress entry is host, host:port or *.domain in lowercase ASCII"
	case CodeTooManyEgress:
		return "egress has at most 64 entries"
	case CodeLabelInvalid:
		return "a runner label is lowercase name or name=value of at most 128 characters"
	case CodeTooManyLabels:
		return "a job has at most 32 runner labels"
	case CodeIsolationNotSupported:
		return "gvisor isolation is not supported yet: use container"
	case CodeIsolationInvalid:
		return "isolation must be container"
	case CodeArtifactNameInvalid:
		return "an artifact name is 1 to 64 of letters, digits, ., _ and -, written as text or as text and matrix values joined with +"
	case CodeArtifactDuplicate:
		return "two artifacts in the pipeline have the same name (add a matrix value to the name)"
	case CodeTooManyArtifacts:
		return "a job has at most 16 artifacts"
	case CodePathInvalid:
		return "a path is relative to the workspace, without .., ~ or a leading /"
	case CodeTooManyPaths:
		return "there are at most 32 paths here"
	case CodePathsRequired:
		return "at least one path is required"
	case CodeRetentionInvalid:
		return "retention is a duration from 1d to 30d"
	case CodeCacheKeyRequired:
		return "cache needs a key"
	case CodeGroupRequired:
		return "concurrency needs a group"
	}
	return ""
}

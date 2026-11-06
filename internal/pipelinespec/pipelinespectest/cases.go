package pipelinespectest

import (
	"fmt"
	"strings"

	"github.com/Khaymah/dhow/internal/jobspec"
	ps "github.com/Khaymah/dhow/internal/pipelinespec"
)

const (
	Digest = "61bb7a95b0f3650f3039c0fcabe07674458cf0d13afa51aade6188bab8215b07"
	Image  = "alpine@sha256:" + Digest
)

type Case struct {
	Name   string
	Source string
	Code   string
}

func Pipeline(jobs string) string {
	return "version: 1\non:\n  push: {}\njobs:\n" + jobs
}

func Job(body string) string {
	return Pipeline("  a:\n    image: " + Image + "\n" + body)
}

func RunJob(stepExtra string) string {
	return Job("    steps:\n      - run: make\n" + stepExtra)
}

func Invalid() []Case {
	out := structureCases()
	out = append(out, policyCases()...)
	out = append(out, fileCases()...)
	return append(out, limitCases()...)
}

func structureCases() []Case {
	img := Image
	return []Case{
		{"cycle", Pipeline("  a:\n    needs: [b]\n    image: " + img + "\n    steps: [{run: x}]\n  b:\n    needs: [a]\n    image: " + img + "\n    steps: [{run: x}]\n"), ps.CodeDAGCycle},
		{"self need", Job("    needs: [a]\n    steps: [{run: x}]\n"), ps.CodeDAGCycle},
		{"unknown need", Job("    needs: [ghost]\n    steps: [{run: x}]\n"), ps.CodeNeedUnknown},
		{"duplicate need", Pipeline("  a:\n    image: " + img + "\n    steps: [{run: x}]\n  b:\n    needs: [a, a]\n    image: " + img + "\n    steps: [{run: x}]\n"), ps.CodeNeedDuplicate},
		{"unknown key", Job("    stepz: []\n    steps: [{run: x}]\n"), ps.CodeUnknownKey},
		{"unknown top key", "version: 1\nservices: {}\non: {push: {}}\njobs: {a: {image: " + img + ", steps: [{run: x}]}}\n", ps.CodeUnknownKey},
		{"alias", "version: 1\non: {push: {}}\njobs:\n  a:\n    image: &i " + img + "\n    steps: [{run: x}]\n  b:\n    image: *i\n    steps: [{run: x}]\n", ps.CodeAliasNotAllowed},
		{"anchor", Job("    env: &e {A: b}\n    steps: [{run: x}]\n"), ps.CodeAnchorNotAllowed},
		{"merge key", Job("    env:\n      <<: {A: b}\n    steps: [{run: x}]\n"), ps.CodeMergeKeyNotAllowed},
		{"custom tag", Job("    env: {A: !secret x}\n    steps: [{run: x}]\n"), ps.CodeTagNotAllowed},
		{"binary tag", Job("    env: {A: !!binary aGk=}\n    steps: [{run: x}]\n"), ps.CodeTagNotAllowed},
		{"duplicate key", Job("    steps: [{run: x}]\n    steps: [{run: y}]\n"), ps.CodeDuplicateKey},
		{"duplicate job", "version: 1\non: {push: {}}\njobs:\n  a: {image: " + img + ", steps: [{run: x}]}\n  a: {image: " + img + ", steps: [{run: x}]}\n", ps.CodeDuplicateKey},
		{"expression in run", RunJob("      - run: echo ${{ git.message }}\n"), ps.CodeRunExpression},
		{"expression in run no spaces", RunJob("      - run: echo ${{git.message}}\n"), ps.CodeRunExpression},
		{"secrets in job if", Job("    if: ${{ secrets.TOKEN == \"x\" }}\n    steps: [{run: x}]\n"), ps.CodeSecretsInIf},
		{"secrets in step if", RunJob("      - run: x\n        if: secrets.TOKEN != \"\"\n"), ps.CodeSecretsInIf},
		{"secret in larger expression", Job("    env: {A: '${{ \"x\" + secrets.T }}'}\n    steps: [{run: x}]\n"), ps.CodeSecretNotWholeValue},
		{"secret in group", "version: 1\non: {push: {}}\nconcurrency: {group: '${{ secrets.T }}'}\njobs: {a: {image: " + img + ", steps: [{run: x}]}}\n", ps.CodeSecretsNotAllowed},
		{"secret in cache key", Job("    steps: [{run: x}]\n    cache: {key: '${{ secrets.T }}', paths: [x]}\n"), ps.CodeSecretsNotAllowed},
		{"matrix too large", Job("    strategy:\n      matrix:\n        a: [1,2,3,4,5,6,7,8]\n        b: [1,2,3,4,5,6,7,8]\n        c: [1,2,3,4,5]\n    steps: [{run: x}]\n"), ps.CodeMatrixTooLarge},
		{"matrix raw too large", Job("    strategy:\n      matrix:\n        a: [1,2,3,4,5,6,7,8]\n        b: [1,2,3,4,5,6,7,8]\n        c: [1,2,3,4,5,6,7,8]\n        d: [1,2,3,4,5,6,7,8]\n    steps: [{run: x}]\n"), ps.CodeMatrixTooLarge},
		{"uses without digest", RunJob("      - uses: ghcr.io/x/y:1\n"), ps.CodeUsesNotPinned},
		{"uses latest", RunJob("      - uses: ghcr.io/x/y\n"), ps.CodeUsesNotPinned},
		{"uses invalid", RunJob("      - uses: 'not an image'\n"), ps.CodeUsesInvalid},
		{"version", "version: 2\non: {push: {}}\njobs: {a: {image: " + img + ", steps: [{run: x}]}}\n", ps.CodeVersionUnsupported},
		{"missing version", "on: {push: {}}\njobs: {a: {image: " + img + ", steps: [{run: x}]}}\n", ps.CodeVersionUnsupported},
		{"no triggers", "version: 1\njobs: {a: {image: " + img + ", steps: [{run: x}]}}\n", ps.CodeTriggersRequired},
		{"schedule", "version: 1\non: {schedule: [{cron: '* * * * *'}]}\njobs: {a: {image: " + img + ", steps: [{run: x}]}}\n", ps.CodeTriggerNotSupported},
		{"artifact trigger without names", "version: 1\non: {artifact: {}}\njobs: {a: {image: " + img + ", steps: [{run: x}]}}\n", ps.CodeArtifactTriggerInvalid},
		{"artifact trigger with a bad name", "version: 1\non: {artifact: {names: ['../x']}}\njobs: {a: {image: " + img + ", steps: [{run: x}]}}\n", ps.CodeArtifactTriggerInvalid},
		{"artifact trigger with a repeated name", "version: 1\non: {artifact: {names: [web, web]}}\njobs: {a: {image: " + img + ", steps: [{run: x}]}}\n", ps.CodeArtifactTriggerInvalid},
		{"artifact trigger with an unknown key", "version: 1\non: {artifact: {names: [web], from: x}}\njobs: {a: {image: " + img + ", steps: [{run: x}]}}\n", ps.CodeUnknownKey},
		{"ingested without on.artifact", "version: 1\non: {push: {}}\njobs: {a: {image: " + img + ", ingested: [web], steps: [{run: x}]}}\n", ps.CodeIngestedNotDeclared},
		{"ingested with an undeclared name", "version: 1\non: {artifact: {names: [web]}}\njobs: {a: {image: " + img + ", ingested: [api], steps: [{run: x}]}}\n", ps.CodeIngestedNotDeclared},
		{"ingested with a bad name", "version: 1\non: {artifact: {names: [web]}}\njobs: {a: {image: " + img + ", ingested: ['../web'], steps: [{run: x}]}}\n", ps.CodeIngestedInvalid},
		{"ingested twice", "version: 1\non: {artifact: {names: [web]}}\njobs: {a: {image: " + img + ", ingested: [web, web], steps: [{run: x}]}}\n", ps.CodeIngestedInvalid},
		{"ingested name produced by a job", "version: 1\non: {artifact: {names: [web]}}\njobs: {a: {image: " + img + ", ingested: [web], steps: [{run: x}]}, b: {image: " + img + ", steps: [{run: x}], artifacts: [{name: web, paths: [dist]}]}}\n", ps.CodeIngestedInvalid},
		{"no jobs", "version: 1\non: {push: {}}\n", ps.CodeJobsRequired},
		{"bad job id", Pipeline("  Build:\n    image: " + img + "\n    steps: [{run: x}]\n"), ps.CodeJobIDInvalid},
		{"no steps", Job("    steps: []\n"), ps.CodeStepsRequired},
		{"run and uses", RunJob("      - run: x\n        uses: ghcr.io/x/y@sha256:" + Digest + "\n"), ps.CodeStepRunOrUses},
		{"neither run nor uses", RunJob("      - name: nothing\n"), ps.CodeStepRunOrUses},
		{"with without uses", RunJob("      - run: x\n        with: {a: b}\n"), ps.CodeWithWithoutUses},
		{"image required", Pipeline("  a:\n    steps: [{run: x}]\n"), ps.CodeImageRequired},
		{"image expression", Pipeline("  a:\n    image: '${{ matrix.i }}'\n    steps: [{run: x}]\n"), ps.CodeExpressionNotAllowed},
		{"env reserved", Job("    env: {DHOW_X: a}\n    steps: [{run: x}]\n"), ps.CodeEnvNameReserved},
		{"env name", Job("    env: {'1A': a}\n    steps: [{run: x}]\n"), ps.CodeEnvNameInvalid},
		{"partial expression", Job("    env: {A: 'v${{ git.tag }}'}\n    steps: [{run: x}]\n"), ps.CodeExpressionSyntax},
		{"cel syntax", Job("    if: ${{ git.branch == }}\n    steps: [{run: x}]\n"), ps.CodeExpressionSyntax},
		{"if not bool", Job("    if: ${{ git.branch }}\n    steps: [{run: x}]\n"), ps.CodeExpressionType},
		{"env not string", Job("    env: {A: '${{ git.is_tag }}'}\n    steps: [{run: x}]\n"), ps.CodeExpressionType},
		{"bad comparison", Job("    if: ${{ git.branch == 1 }}\n    steps: [{run: x}]\n"), ps.CodeExpressionType},
	}
}

func policyCases() []Case {
	img := Image
	return []Case{
		{"unknown variable", Job("    if: ${{ github.ref == \"x\" }}\n    steps: [{run: x}]\n"), ps.CodeExpressionUnknownName},
		{"undeclared input", Job("    if: ${{ inputs.x == \"y\" }}\n    steps: [{run: x}]\n"), ps.CodeExpressionUnknownName},
		{"needs not listed", Pipeline("  a:\n    image: " + img + "\n    steps: [{run: x}]\n  b:\n    if: ${{ needs.a.outputs.v == \"1\" }}\n    image: " + img + "\n    steps: [{run: x}]\n"), ps.CodeExpressionUnknownName},
		{"env in job if", Job("    if: ${{ env.A == \"1\" }}\n    steps: [{run: x}]\n"), ps.CodeExpressionUnknownName},
		{"hash_files outside cache", Job("    env: {A: '${{ hash_files(\"x\") }}'}\n    steps: [{run: x}]\n"), ps.CodeExpressionUnknownName},
		{"long expression", Job("    if: ${{ " + strings.Repeat("true && ", 300) + "true }}\n    steps: [{run: x}]\n"), ps.CodeExpressionTooLong},
		{"bad timeout", Job("    timeout: 2y\n    steps: [{run: x}]\n"), ps.CodeTimeoutInvalid},
		{"timeout too long", Job("    timeout: 25h\n    steps: [{run: x}]\n"), ps.CodeTimeoutInvalid},
		{"step longer than job", Job("    timeout: 10m\n    steps: [{run: x, timeout: 11m}]\n"), ps.CodeTimeoutInvalid},
		{"cpu", Job("    resources: {cpu: 100}\n    steps: [{run: x}]\n"), ps.CodeResourceInvalid},
		{"memory unit", Job("    resources: {memory: 4GB}\n    steps: [{run: x}]\n"), ps.CodeResourceInvalid},
		{"egress ip", Job("    network: {egress: [10.0.0.1]}\n    steps: [{run: x}]\n"), ps.CodeEgressInvalid},
		{"label", Job("    runner: {labels: [Bad Label]}\n    steps: [{run: x}]\n"), ps.CodeLabelInvalid},
		{"gvisor", Job("    runner: {isolation: gvisor}\n    steps: [{run: x}]\n"), ps.CodeIsolationNotSupported},
		{"microvm", Job("    runner: {isolation: microvm}\n    steps: [{run: x}]\n"), ps.CodeIsolationInvalid},
		{"pattern", "version: 1\non: {push: {branches: ['feat/[a-z]']}}\njobs: {a: {image: " + img + ", steps: [{run: x}]}}\n", ps.CodePatternInvalid},
		{"pattern stars", "version: 1\non: {push: {branches: ['a**']}}\njobs: {a: {image: " + img + ", steps: [{run: x}]}}\n", ps.CodePatternInvalid},
		{"choice default", "version: 1\non: {manual: {inputs: {t: {type: choice, options: [a], default: b}}}}\njobs: {a: {image: " + img + ", steps: [{run: x}]}}\n", ps.CodeInputInvalid},
		{"boolean default", "version: 1\non: {manual: {inputs: {t: {type: boolean, default: yes}}}}\njobs: {a: {image: " + img + ", steps: [{run: x}]}}\n", ps.CodeInputInvalid},
		{"artifact path", Job("    steps: [{run: x}]\n    artifacts: [{name: x, paths: [../etc]}]\n"), ps.CodePathInvalid},
		{"artifact absolute", Job("    steps: [{run: x}]\n    artifacts: [{name: x, paths: [/etc]}]\n"), ps.CodePathInvalid},
		{"cache home", Job("    steps: [{run: x}]\n    cache: {key: k, paths: [~/.npm]}\n"), ps.CodePathInvalid},
		{"artifact name", Job("    steps: [{run: x}]\n    artifacts: [{name: 'a b', paths: [x]}]\n"), ps.CodeArtifactNameInvalid},
		{"artifact duplicate in matrix", Job("    strategy: {matrix: {os: [a, b]}}\n    steps: [{run: x}]\n    artifacts: [{name: out, paths: [x]}]\n"), ps.CodeArtifactDuplicate},
		{"retention", Job("    steps: [{run: x}]\n    artifacts: [{name: x, paths: [x], retention: 90d}]\n"), ps.CodeRetentionInvalid},
		{"cache key", Job("    steps: [{run: x}]\n    cache: {paths: [x]}\n"), ps.CodeCacheKeyRequired},
		{"group", "version: 1\non: {push: {}}\nconcurrency: {cancel_in_progress: true}\njobs: {a: {image: " + img + ", steps: [{run: x}]}}\n", ps.CodeGroupRequired},
		{"matrix empty axis", Job("    strategy: {matrix: {os: []}}\n    steps: [{run: x}]\n"), ps.CodeMatrixEmpty},
		{"matrix all excluded", Job("    strategy: {matrix: {os: [a], exclude: [{os: a}]}}\n    steps: [{run: x}]\n"), ps.CodeMatrixEmpty},
		{"matrix key", Job("    strategy: {matrix: {'node-version': [1]}}\n    steps: [{run: x}]\n"), ps.CodeMatrixKeyInvalid},
		{"matrix nested value", Job("    strategy: {matrix: {os: [[a]]}}\n    steps: [{run: x}]\n"), ps.CodeTypeMismatch},
		{"matrix include keys", Job("    strategy: {matrix: {os: [a], include: [{arch: b}]}}\n    steps: [{run: x}]\n"), ps.CodeMatrixEntryInvalid},
		{"matrix exclude keys", Job("    strategy: {matrix: {os: [a, b], exclude: [{arch: b}]}}\n    steps: [{run: x}]\n"), ps.CodeMatrixEntryInvalid},
		{"matrix duplicate", Job("    strategy: {matrix: {os: [a, a]}}\n    steps: [{run: x}]\n"), ps.CodeMatrixDuplicate},
		{"max parallel", Job("    strategy: {max_parallel: 300}\n    steps: [{run: x}]\n"), ps.CodeMaxParallelInvalid},
		{"environment name", Job("    environment: Prod Env\n    steps: [{run: x}]\n"), ps.CodeEnvironmentInvalid},
		{"type mismatch", Job("    steps: {run: x}\n"), ps.CodeTypeMismatch},
		{"bool as yes", "version: 1\non: {push: {}}\nconcurrency: {group: g, cancel_in_progress: yes}\njobs: {a: {image: " + img + ", steps: [{run: x}]}}\n", ps.CodeTypeMismatch},
		{"step id duplicate", RunJob("      - {run: y, id: s}\n      - {run: z, id: s}\n"), ps.CodeStepIDDuplicate},
		{"plugin input name", RunJob("      - uses: ghcr.io/x/y@sha256:" + Digest + "\n        with: {Bad-Name: x}\n"), ps.CodeInputInvalid},
		{"name expression", Job("    name: '${{ git.ref }}'\n    steps: [{run: x}]\n"), ps.CodeNameInvalid},
	}
}

func fileCases() []Case {
	return []Case{
		{"empty file", "", ps.CodeFileEmpty},
		{"comment only", "# nothing\n", ps.CodeFileEmpty},
		{"two documents", RunJob("") + "---\nversion: 1\n", ps.CodeMultipleDocuments},
		{"syntax", "version: 1\non: [push\n", ps.CodeYAMLSyntax},
		{"complex key", "? [a]\n: b\n", ps.CodeKeyInvalid},
		{"root list", "- a\n", ps.CodeTypeMismatch},
		{"file too large", strings.Repeat("#", ps.MaxFileBytes+1), ps.CodeFileTooLarge},
		{"deep nesting", "a: " + strings.Repeat("[", 40) + strings.Repeat("]", 40) + "\n", ps.CodeDepthExceeded},
		{"deep mappings", strings.Repeat("{a: ", 40) + "1" + strings.Repeat("}", 40) + "\n", ps.CodeDepthExceeded},
		{"too many nodes", "a: [" + strings.Repeat("1,", ps.MaxNodes) + "1]\n", ps.CodeTooManyNodes},
		{"scalar too large", "name: " + strings.Repeat("x", ps.MaxScalarBytes+1) + "\n", ps.CodeScalarTooLarge},
	}
}

func limitCases() []Case {
	var jobs strings.Builder
	for i := range ps.MaxJobs + 1 {
		fmt.Fprintf(&jobs, "  j%d:\n    image: %s\n    steps: [{run: x}]\n", i, Image)
	}
	values := make([]string, 0, 200)
	for i := range 200 {
		values = append(values, fmt.Sprint(i))
	}
	var matrix strings.Builder
	for i := range 2 {
		fmt.Fprintf(&matrix, "  j%d:\n    image: %s\n    strategy: {matrix: {n: [%s]}}\n    steps: [{run: x}]\n", i, Image, strings.Join(values, ","))
	}
	steps := strings.Repeat("      - run: x\n", jobspec.MaxSteps+1)
	return []Case{
		{"too many jobs", Pipeline(jobs.String()), ps.CodeTooManyJobs},
		{"too many run jobs", Pipeline(matrix.String()), ps.CodeTooManyRunJobs},
		{"too many steps", Job("    steps:\n" + steps), ps.CodeTooManySteps},
	}
}

package pipelinespec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/jobspec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

func fullExample(t *testing.T) []byte {
	t.Helper()
	blocks := yamlBlocks(t, "docs/en/pipeline-spec.md")
	require.NotEmpty(t, blocks)
	return []byte(blocks[0])
}

func compileSource(t *testing.T, src []byte) *Plan {
	t.Helper()
	spec, issues := Parse(src)
	require.NotNil(t, spec, "%v", issues)
	plan, err := Compile(spec)
	require.NoError(t, err)
	return plan
}

func jobByKey(t *testing.T, plan *Plan, key string) *RunJob {
	t.Helper()
	for _, j := range plan.Jobs {
		if j.Key == key {
			return j
		}
	}
	require.Failf(t, "missing run job", "%s", key)
	return nil
}

func TestCompileFullExample(t *testing.T) {
	plan := compileSource(t, fullExample(t))
	keys := make([]string, 0, len(plan.Jobs))
	for _, j := range plan.Jobs {
		keys = append(keys, j.Key)
	}
	assert.Equal(t, []string{
		"test (node=20)", "test (node=22)", "build", "deploy-staging", "deploy-production", "migrate-db",
	}, keys)
	require.NotNil(t, plan.Concurrency)
	assert.Equal(t, `project.name + "-" + git.ref`, plan.Concurrency.Group.Expression)

	test := jobByKey(t, plan, "test (node=20)")
	assert.Equal(t, "docker.io/library/node:22-bookworm@sha256:61bb7a95b0f3650f3039c0fcabe07674458cf0d13afa51aade6188bab8215b07", test.Spec.GetImage())
	assert.Equal(t, "matrix.node", test.Expressions["Env.NODE_MAJOR"])
	assert.NotContains(t, test.Spec.GetEnv(), "NODE_MAJOR")
	assert.Equal(t, int32(20*60), test.Spec.GetTimeoutSeconds())
	assert.Equal(t, []string{"amd64", "linux"}, test.Spec.GetLabels())
	require.NotNil(t, test.Cache)
	assert.Contains(t, test.Cache.Key.Expression, "hash_files")

	build := jobByKey(t, plan, "build")
	assert.Equal(t, []string{"test (node=20)", "test (node=22)"}, build.Needs)
	assert.Equal(t, int64(2000), build.Spec.GetResources().GetCpuMillis())
	assert.Equal(t, int64(4<<30), build.Spec.GetResources().GetMemoryBytes())
	assert.Equal(t, int64(jobspec.DefaultDiskBytes), build.Spec.GetResources().GetDiskBytes())
	assert.Equal(t, []string{"registry.npmjs.org"}, build.Spec.GetNetwork().GetEgress())
	assert.Equal(t, "npm ci && npm run build", build.Spec.GetSteps()[0].GetName())
	assert.Equal(t, []ArtifactPlan{{Name: "web", Paths: []string{"dist/", "package.json", "package-lock.json"}}}, build.Artifacts)

	staging := jobByKey(t, plan, "deploy-staging")
	assert.Empty(t, staging.Spec.GetImage())
	require.NotNil(t, staging.Steps[0].Plugin)
	assert.Equal(t, "5", staging.Steps[0].Plugin.Inputs["keep_releases"])
	assert.Equal(t, "khaymah/dhow-deploy-ssh", staging.Spec.GetSteps()[0].GetName())
	assert.Equal(t, "staging", staging.Environment)

	migrate := jobByKey(t, plan, "migrate-db")
	require.Len(t, migrate.Spec.GetSecrets(), 1)
	ref := migrate.Spec.GetSecrets()[0]
	assert.Equal(t, "DATABASE_URL", ref.GetName())
	assert.Equal(t, "DATABASE_URL", ref.GetAsEnv())
	assert.Equal(t, []int32{0}, ref.GetStepIndexes())
	assert.NotContains(t, migrate.Spec.GetSteps()[0].GetEnv(), "DATABASE_URL")

	codes := map[string]bool{}
	for _, w := range plan.Warnings {
		codes[w.Code] = true
		assert.Equal(t, SeverityWarning, w.Severity)
	}
	assert.False(t, codes[CodeSecretsUnavailable], "secrets are no longer a warning")
	assert.False(t, codes[CodeEnvironmentsDeferred], "environments are no longer a warning")
}

func TestCompileIsDeterministic(t *testing.T) {
	src := fullExample(t)
	first := compileSource(t, src)
	for range 20 {
		again := compileSource(t, src)
		require.Len(t, again.Jobs, len(first.Jobs))
		for i := range first.Jobs {
			assert.Equal(t, first.Jobs[i].Key, again.Jobs[i].Key)
			assert.Equal(t, first.Jobs[i].Digest, again.Jobs[i].Digest)
			a, err := jobspec.Digest(first.Jobs[i].Spec)
			require.NoError(t, err)
			b, err := jobspec.Digest(again.Jobs[i].Spec)
			require.NoError(t, err)
			assert.Equal(t, a, b)
		}
	}
	assert.NotEqual(t, first.Jobs[0].Digest, first.Jobs[1].Digest)
}

func TestCompileSpecsPassJobSpecValidation(t *testing.T) {
	plan := compileSource(t, fullExample(t))
	for _, j := range plan.Jobs {
		if len(j.Expressions) == 0 {
			assert.NoError(t, jobspec.Validate(j.Spec), j.Key)
		}
		assert.Equal(t, runnerv1.Isolation_ISOLATION_CONTAINER, j.Spec.GetIsolation())
	}
}

func TestCompileRefusesInvalidSpec(t *testing.T) {
	spec, issues := Parse([]byte("version: 2\non: {push: {}}\njobs: {a: {image: alpine, steps: [{run: x}]}}\n"))
	require.NotNil(t, spec, "%v", issues)
	_, err := Compile(spec)
	var perr *Error
	require.ErrorAs(t, err, &perr)
	assert.Equal(t, CodeVersionUnsupported, perr.Issues[0].Code)
}

func TestCompileJobLevelSecretCoversEveryStep(t *testing.T) {
	plan := compileSource(t, []byte(`version: 1
on: {manual: {}}
jobs:
  a:
    image: alpine@sha256:61bb7a95b0f3650f3039c0fcabe07674458cf0d13afa51aade6188bab8215b07
    env:
      TOKEN: ${{ secrets.TOKEN }}
      PLAIN: "1"
    steps:
      - run: one
      - run: two
`))
	j := plan.Jobs[0]
	require.Len(t, j.Spec.GetSecrets(), 1)
	assert.Empty(t, j.Spec.GetSecrets()[0].GetStepIndexes())
	assert.Equal(t, map[string]string{"PLAIN": "1"}, j.Spec.GetEnv())
}

func TestAPluginStepCarriesItsImageInputsAndStepScopedSecrets(t *testing.T) {
	plugin := "ghcr.io/team/notify@sha256:" + digest
	plan := compileSource(t, []byte(pipeline("  a:\n    image: "+img+"\n    steps:\n      - run: make\n      - uses: "+plugin+
		"\n        with:\n          channel: ops\n          token: ${{ secrets.SLACK_TOKEN }}\n")))
	a := jobByKey(t, plan, "a")
	step := a.Spec.GetSteps()[1]
	require.NotNil(t, step.GetPlugin(), "the job spec carries the plugin")
	assert.Equal(t, plugin, step.GetPlugin().GetImage())
	assert.Equal(t, map[string]string{"channel": "ops"}, step.GetPlugin().GetInputs())
	require.Len(t, a.Spec.GetSecrets(), 1)
	ref := a.Spec.GetSecrets()[0]
	assert.Equal(t, "SLACK_TOKEN", ref.GetName())
	assert.Equal(t, "DHOW_INPUT_TOKEN", ref.GetAsEnv())
	assert.Equal(t, []int32{1}, ref.GetStepIndexes(), "only the plugin step gets the secret")
	assert.NoError(t, jobspec.Validate(a.Spec))
}

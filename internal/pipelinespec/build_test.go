package pipelinespec

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/jobspec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

var onMain = Context{Git: Git{Branch: "main", Ref: "refs/heads/main"}}

const skippedStep = "      - run: echo skipped\n        if: ${{ git.branch == 'nope' }}\n"

func builtJob(t *testing.T, steps string) *runnerv1.JobSpec {
	t.Helper()
	plan := compileSource(t, []byte(job("    steps:\n"+steps)))
	require.Len(t, plan.Jobs, 1)
	spec, err := plan.Jobs[0].Build(context.Background(), onMain)
	require.NoError(t, err)
	require.Empty(t, jobspec.Validate(spec))
	return spec
}

func secretSteps(spec *runnerv1.JobSpec) map[string][]int32 {
	out := map[string][]int32{}
	for _, s := range spec.GetSecrets() {
		out[s.GetName()+"="+s.GetAsEnv()] = s.GetStepIndexes()
	}
	return out
}

func TestASkippedStepDoesNotShiftAStepSecretOntoAPlugin(t *testing.T) {
	spec := builtJob(t, skippedStep+
		"      - run: ./deploy.sh\n        env:\n          TOKEN: ${{ secrets.PROD_TOKEN }}\n"+
		"      - uses: ghcr.io/thirdparty/notify@sha256:"+digest+"\n        with:\n          channel: ops\n")
	require.Len(t, spec.GetSteps(), 2)
	assert.Equal(t, "./deploy.sh", spec.GetSteps()[0].GetRun())
	require.NotNil(t, spec.GetSteps()[1].GetPlugin())
	assert.Equal(t, map[string][]int32{"PROD_TOKEN=TOKEN": {0}}, secretSteps(spec))
}

func TestASkippedStepDoesNotShiftAPluginInputSecretOntoAnotherPlugin(t *testing.T) {
	spec := builtJob(t, skippedStep+
		"      - uses: ghcr.io/trusted/deploy@sha256:"+digest+"\n        with:\n          token: ${{ secrets.DEPLOY_TOKEN }}\n"+
		"      - uses: ghcr.io/thirdparty/notify@sha256:"+digest+"\n        with:\n          channel: ops\n")
	require.Len(t, spec.GetSteps(), 2)
	assert.Contains(t, spec.GetSteps()[0].GetPlugin().GetImage(), "trusted/deploy")
	assert.Equal(t, map[string][]int32{"DEPLOY_TOKEN=DHOW_INPUT_TOKEN": {0}}, secretSteps(spec))
}

func TestASecretOfASkippedStepIsDropped(t *testing.T) {
	spec := builtJob(t, "      - run: echo first\n"+
		"      - run: ./deploy.sh\n        if: ${{ git.branch == 'nope' }}\n        env:\n          TOKEN: ${{ secrets.PROD_TOKEN }}\n"+
		"      - uses: ghcr.io/thirdparty/notify@sha256:"+digest+"\n        with:\n          key: ${{ secrets.NOTIFY_KEY }}\n")
	require.Len(t, spec.GetSteps(), 2)
	assert.Equal(t, map[string][]int32{"NOTIFY_KEY=DHOW_INPUT_KEY": {1}}, secretSteps(spec))
}

func TestAJobSecretKeepsNoStepIndexesWhenAStepIsSkipped(t *testing.T) {
	plan := compileSource(t, []byte(job("    env:\n      TOKEN: ${{ secrets.JOB_TOKEN }}\n    steps:\n"+skippedStep+"      - run: make\n")))
	spec, err := plan.Jobs[0].Build(context.Background(), onMain)
	require.NoError(t, err)
	require.Len(t, spec.GetSteps(), 1)
	assert.Equal(t, map[string][]int32{"JOB_TOKEN=TOKEN": nil}, secretSteps(spec))
}

func TestBuildRefusesAPluginStepHoldingASecretItDidNotDeclare(t *testing.T) {
	tamper := map[string]func(*RunJob){
		"step env secret moved onto the plugin": func(r *RunJob) { r.Spec.Secrets[0].StepIndexes = []int32{1} },
		"plugin input renamed": func(r *RunJob) {
			r.Spec.Secrets = append(r.Spec.Secrets, &runnerv1.SecretRef{
				Name: "OTHER", Target: &runnerv1.SecretRef_AsEnv{AsEnv: jobspec.PluginInputEnvPrefix + "CHANNEL"}, StepIndexes: []int32{1},
			})
		},
		"plugin input given another secret":  func(r *RunJob) { r.Spec.Secrets[1].Name = "PROD_TOKEN" },
		"step secret shared with the plugin": func(r *RunJob) { r.Spec.Secrets[0].StepIndexes = []int32{0, 1} },
	}
	for name, change := range tamper {
		t.Run(name, func(t *testing.T) {
			plan := compileSource(t, []byte(job("    steps:\n"+
				"      - run: ./deploy.sh\n        env:\n          TOKEN: ${{ secrets.PROD_TOKEN }}\n"+
				"      - uses: ghcr.io/thirdparty/notify@sha256:"+digest+"\n        with:\n          key: ${{ secrets.NOTIFY_KEY }}\n          channel: ops\n")))
			rj := plan.Jobs[0]
			require.Len(t, rj.Spec.GetSecrets(), 2)
			_, err := rj.Build(context.Background(), onMain)
			require.NoError(t, err)
			change(rj)
			_, err = rj.Build(context.Background(), onMain)
			require.ErrorIs(t, err, ErrSecretScope)
		})
	}
}

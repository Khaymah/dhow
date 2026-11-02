//go:build dhowtest

package app_test

import (
	"bytes"
	"encoding/base64"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	runnertest "github.com/Khaymah/dhow/apps/runner/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/runner/internal/Config"
	sandbox "github.com/Khaymah/dhow/apps/runner/internal/Sandbox"
	fake "github.com/Khaymah/dhow/apps/runner/internal/Sandbox/Fake"
	"github.com/Khaymah/dhow/internal/jobspec"
	"github.com/Khaymah/dhow/internal/runnersig"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const (
	checkoutImage = "ghcr.io/khaymah/dhow-git@sha256:1111111111111111111111111111111111111111111111111111111111111111"
	pluginImage   = "registry.example.com/tools/notify@sha256:2222222222222222222222222222222222222222222222222222222222222222"
	checkoutSha   = "0123456789abcdef0123456789abcdef01234567"
	deployKey     = "fake-deploy-key-c2VjcmV0LWRlcGxveS1rZXktbWF0ZXJpYWw"
	manifest      = `{"Version":1,"Entrypoint":["/usr/local/bin/notify","--quiet"],"Inputs":{"channel":{"Required":true},"message":{"Default":"done"}},"Outputs":["url"]}`
)

func knownHostLine() string {
	blob := append([]byte{0, 0, 0, 11}, "ssh-ed25519"...)
	blob = append(blob, make([]byte, 36)...)
	return "[git.example.com]:2222 ssh-ed25519 " + base64.StdEncoding.EncodeToString(blob)
}

func checkoutSpec() *runnerv1.JobSpec {
	spec := runnertest.Spec(
		&runnerv1.Step{Name: "build", Run: "make build"},
		&runnerv1.Step{Name: "notify", Id: "notify", Plugin: &runnerv1.Plugin{Image: pluginImage, Inputs: map[string]string{"channel": "#ops"}}},
		&runnerv1.Step{Name: "test", Run: "make test"},
	)
	spec.Checkout = &runnerv1.Checkout{
		RepositoryUrl: "ssh://git@git.example.com:2222/team/app.git", Ref: "refs/heads/main", Sha: checkoutSha,
		Transport: runnerv1.CheckoutTransport_CHECKOUT_TRANSPORT_SSH, CredentialSecret: jobspec.CheckoutSecret,
		KnownHost: knownHostLine(), Depth: 1,
	}
	spec.Secrets = []*runnerv1.SecretRef{{Name: jobspec.CheckoutSecret, Target: &runnerv1.SecretRef_AsFile{AsFile: jobspec.CheckoutSecretFile}}}
	spec.Network = &runnerv1.Network{Egress: []string{"proxy.golang.org"}}
	return spec
}

func checkoutRunner(t *testing.T, srv *runnertest.Server) *runnertest.Runner {
	t.Helper()
	r := runnertest.Register(t, srv, srv.URL)
	r.Env[config.EnvCheckoutImage] = checkoutImage
	return r
}

func images() map[string]map[string]string {
	return map[string]map[string]string{pluginImage: {sandbox.PluginManifestLabel: manifest}}
}

func execs(events []fake.Event) []fake.Event {
	var out []fake.Event
	for _, e := range events {
		if e.Kind == fake.EventExec {
			out = append(out, e)
		}
	}
	return out
}

func TestACheckoutAndAPluginRunInSandboxesOfTheirOwnOnOneWorkspace(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := checkoutRunner(t, srv)
	srv.Enqueue(checkoutSpec(), map[string][]byte{jobspec.CheckoutSecret: []byte(deployKey)})
	running := r.Start(fake.Recording{
		Images: images(),
		Steps:  []fake.Step{{}, {Outputs: "url=https://chat.example.com/m/1\n"}, {}},
	}, runnertest.StartOptions{})
	out := running.Next(t, finishWithin)
	require.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, out.Result, out.Reason.String())

	events := running.Driver.Events()
	var prepares []fake.Event
	for _, e := range events {
		if e.Kind == fake.EventPrepare {
			prepares = append(prepares, e)
		}
	}
	require.Len(t, prepares, 4)
	assert.Equal(t, []sandbox.Kind{sandbox.KindCheckout, sandbox.KindJob, sandbox.KindPlugin, sandbox.KindJob},
		[]sandbox.Kind{prepares[0].Box, prepares[1].Box, prepares[2].Box, prepares[3].Box})
	assert.Equal(t, []string{checkoutImage, runnertest.TestImage, pluginImage, runnertest.TestImage},
		[]string{prepares[0].Image, prepares[1].Image, prepares[2].Image, prepares[3].Image})
	assert.Equal(t, []int{0, 1, 2, 3}, []int{prepares[0].Seq, prepares[1].Seq, prepares[2].Seq, prepares[3].Seq})
	k := kinds(events)
	assert.Equal(t, fake.EventWorkspace, k[1], "the workspace comes before any sandbox")
	assert.Equal(t, fake.EventWorkspaceDestroy, k[len(k)-1], "the workspace goes after the last sandbox")
	assert.Equal(t, 1, strings.Count(strings.Join(eventNames(k), " "), string(fake.EventWorkspace)+" "))

	steps := execs(events)
	require.Len(t, steps, 4)
	co := steps[0]
	assert.Equal(t, sandbox.CheckoutStepIndex, co.Step)
	assert.Equal(t, []string{sandbox.AgentPath, "__checkout"}, co.Argv)
	assert.Equal(t, "ssh://git@git.example.com:2222/team/app.git", co.Env["DHOW_CHECKOUT_URL"])
	assert.Equal(t, checkoutSha, co.Env["DHOW_CHECKOUT_SHA"])
	assert.Equal(t, []string{"git-credential", "known_hosts"}, co.FileNames)

	for _, e := range steps[1:] {
		assert.NotContains(t, e.FileNames, "git-credential", "step %d", e.Step)
		for name, v := range e.Env {
			assert.NotContains(t, v, "c2VjcmV0LWRlcGxveS1rZXk", "the checkout credential reached %s of step %d", name, e.Step)
			assert.False(t, strings.HasPrefix(name, "DHOW_CHECKOUT_"), name)
		}
		for _, f := range e.FileData {
			assert.False(t, bytes.Contains(f, []byte("c2VjcmV0LWRlcGxveS1rZXk")))
		}
	}
	plug := steps[2]
	assert.Equal(t, int32(1), plug.Step)
	assert.Equal(t, []string{"/usr/local/bin/notify", "--quiet"}, plug.Argv)
	assert.Equal(t, "#ops", plug.Env["DHOW_INPUT_CHANNEL"])
	assert.Equal(t, "done", plug.Env["DHOW_INPUT_MESSAGE"])
	assert.NotEmpty(t, plug.Env["HTTPS_PROXY"], "the plugin uses the job's egress")
	assert.Empty(t, steps[1].Argv, "a run step goes through the shell")

	lease := srv.Leases()[0]
	require.NotEmpty(t, lease.Completes)
	assert.Equal(t, map[string]string{"url": "https://chat.example.com/m/1"}, lease.Completes[len(lease.Completes)-1].GetOutputs())
}

func eventNames(k []fake.EventKind) []string {
	out := make([]string, 0, len(k))
	for _, e := range k {
		out = append(out, string(e))
	}
	return out
}

func TestTheCheckoutResultDecidesTheFailureReason(t *testing.T) {
	tests := []struct {
		exit int32
		want runnerv1.FailureReason
	}{
		{3, runnerv1.FailureReason_FAILURE_REASON_CHECKOUT_REF_MOVED},
		{4, runnerv1.FailureReason_FAILURE_REASON_HOST_KEY_MISMATCH},
		{5, runnerv1.FailureReason_FAILURE_REASON_CHECKOUT_DISK_FULL},
		{128, runnerv1.FailureReason_FAILURE_REASON_CHECKOUT_FAILED},
	}
	for _, tt := range tests {
		t.Run(tt.want.String(), func(t *testing.T) {
			srv := runnertest.NewServer(t)
			r := checkoutRunner(t, srv)
			srv.Enqueue(checkoutSpec(), map[string][]byte{jobspec.CheckoutSecret: []byte(deployKey)})
			running := r.Start(fake.Recording{Images: images(), Checkout: &fake.Step{ExitCode: tt.exit}}, runnertest.StartOptions{})
			out := running.Next(t, finishWithin)
			assert.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_FAILED, out.Result)
			assert.Equal(t, tt.want, out.Reason)
			assert.Len(t, execs(running.Driver.Events()), 1, "no step runs after a failed checkout")
			assert.Equal(t, fake.EventWorkspaceDestroy, kinds(running.Driver.Events())[len(running.Driver.Events())-1])
		})
	}
}

func TestPluginInputsAreCheckedAgainstTheManifestBeforeAnythingRuns(t *testing.T) {
	tests := []struct {
		name   string
		inputs map[string]string
		labels map[string]string
		want   runnerv1.FailureReason
	}{
		{
			"an input the plugin does not declare",
			map[string]string{"channel": "#ops", "token": "x"},
			map[string]string{sandbox.PluginManifestLabel: manifest},
			runnerv1.FailureReason_FAILURE_REASON_PLUGIN_INPUT_INVALID,
		},
		{
			"a required input left out",
			map[string]string{"message": "x"},
			map[string]string{sandbox.PluginManifestLabel: manifest},
			runnerv1.FailureReason_FAILURE_REASON_PLUGIN_INPUT_INVALID,
		},
		{"no manifest", map[string]string{"channel": "#ops"}, nil, runnerv1.FailureReason_FAILURE_REASON_PLUGIN_MANIFEST_INVALID},
		{
			"a shell entrypoint",
			map[string]string{"channel": "#ops"},
			map[string]string{sandbox.PluginManifestLabel: `{"Version":1,"Entrypoint":["sh","-c","x"]}`},
			runnerv1.FailureReason_FAILURE_REASON_PLUGIN_MANIFEST_INVALID,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := runnertest.NewServer(t)
			r := runnertest.Register(t, srv, srv.URL)
			spec := runnertest.Spec(&runnerv1.Step{Name: "notify", Plugin: &runnerv1.Plugin{Image: pluginImage, Inputs: tt.inputs}})
			srv.Enqueue(spec, nil)
			running := r.Start(fake.Recording{Images: map[string]map[string]string{pluginImage: tt.labels}}, runnertest.StartOptions{})
			out := running.Next(t, finishWithin)
			assert.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_FAILED, out.Result)
			assert.Equal(t, tt.want, out.Reason)
			assert.Empty(t, execs(running.Driver.Events()), "the plugin never ran")
			assert.Equal(t, tt.want.String(), srv.Leases()[0].Steps[0], "the step ends with the reason")
		})
	}
}

func TestOutputsOutsideTheLimitsFailTheStep(t *testing.T) {
	tests := []struct {
		name string
		step fake.Step
	}{
		{"not key=value", fake.Step{Outputs: "just text\n"}},
		{"a key the plugin does not declare", fake.Step{Outputs: "token=x\n"}},
		{"a file the agent refused", fake.Step{BadOutputs: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := runnertest.NewServer(t)
			r := runnertest.Register(t, srv, srv.URL)
			spec := runnertest.Spec(&runnerv1.Step{Name: "notify", Plugin: &runnerv1.Plugin{Image: pluginImage, Inputs: map[string]string{"channel": "#ops"}}})
			srv.Enqueue(spec, nil)
			running := r.Start(fake.Recording{Images: images(), Steps: []fake.Step{tt.step}}, runnertest.StartOptions{})
			out := running.Next(t, finishWithin)
			assert.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_FAILED, out.Result)
			assert.Equal(t, runnerv1.FailureReason_FAILURE_REASON_STEP_FAILED, out.Reason)
			for _, c := range srv.Leases()[0].Completes {
				assert.Empty(t, c.GetOutputs())
			}
		})
	}
}

func TestOutputsAreMasked(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := runnertest.Register(t, srv, srv.URL)
	spec := runnertest.Spec(&runnerv1.Step{Name: "echo", Run: "echo"})
	spec.Secrets = []*runnerv1.SecretRef{{Name: "API_TOKEN", Target: &runnerv1.SecretRef_AsEnv{AsEnv: "API_TOKEN"}}}
	srv.Enqueue(spec, map[string][]byte{"API_TOKEN": []byte("s3cr3t-value-123")})
	running := r.Start(fake.Recording{Steps: []fake.Step{{Outputs: "leak=prefix-s3cr3t-value-123\n"}}}, runnertest.StartOptions{})
	require.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, running.Next(t, finishWithin).Result)
	got := srv.Leases()[0].Completes[0].GetOutputs()["leak"]
	assert.NotContains(t, got, "s3cr3t-value-123")
	assert.True(t, strings.HasPrefix(got, "prefix-"), got)
}

func TestARunnerWithoutTheCheckoutImageNeitherClaimsNorRunsACheckout(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := runnertest.Register(t, srv, srv.URL)
	registered, _ := srv.RunnerFeatures()
	assert.Equal(t, []string{runnersig.FeaturePlugins}, registered, "a test build has no baked checkout image")
	srv.Enqueue(checkoutSpec(), map[string][]byte{jobspec.CheckoutSecret: []byte(deployKey)})
	running := r.Start(fake.Recording{Images: images()}, runnertest.StartOptions{})
	out := running.Next(t, finishWithin)
	assert.Equal(t, runnerv1.FailureReason_FAILURE_REASON_SPEC_REJECTED, out.Reason)
	assert.Empty(t, execs(running.Driver.Events()))
	_, signed := srv.RunnerFeatures()
	assert.Equal(t, []string{runnersig.FeaturePlugins}, signed)

	srv2 := runnertest.NewServer(t)
	r2 := checkoutRunner(t, srv2)
	running2 := r2.Start(fake.Recording{}, runnertest.StartOptions{})
	srv2.Enqueue(runnertest.Spec(), nil)
	require.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, running2.Next(t, finishWithin).Result)
	_, signed = srv2.RunnerFeatures()
	assert.True(t, slices.Equal([]string{runnersig.FeatureCheckout, runnersig.FeaturePlugins}, signed), "%v", signed)
}

func TestAPluginGetsOnlyItsInputsItsOutputFileAndTheSecretsOfItsStep(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := runnertest.Register(t, srv, srv.URL)
	spec := runnertest.Spec(
		&runnerv1.Step{Name: "build", Run: "make build"},
		&runnerv1.Step{Name: "notify", Plugin: &runnerv1.Plugin{Image: pluginImage, Inputs: map[string]string{"channel": "#ops"}}},
	)
	spec.Env = map[string]string{"JOB_SETTING": "job-wide"}
	spec.Secrets = []*runnerv1.SecretRef{
		{Name: "DEPLOY_TOKEN", Target: &runnerv1.SecretRef_AsEnv{AsEnv: "DEPLOY_TOKEN"}},
		{Name: "CHAT_TOKEN", Target: &runnerv1.SecretRef_AsEnv{AsEnv: "DHOW_INPUT_TOKEN"}, StepIndexes: []int32{1}},
	}
	srv.Enqueue(spec, map[string][]byte{"DEPLOY_TOKEN": []byte("job-wide-deploy-secret"), "CHAT_TOKEN": []byte("chat-secret-for-notify")})
	labels := map[string]map[string]string{pluginImage: {sandbox.PluginManifestLabel: `{"Version":1,"Entrypoint":["/notify"],"Inputs":{"channel":{"Required":true},"token":{"Required":true}}}`}}
	running := r.Start(fake.Recording{Images: labels}, runnertest.StartOptions{})
	out := running.Next(t, finishWithin)
	require.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, out.Result, out.Reason.String())
	steps := execs(running.Driver.Events())
	require.Len(t, steps, 2)
	assert.Equal(t, "job-wide-deploy-secret", steps[0].Env["DEPLOY_TOKEN"], "a run step still gets the job's secret")
	plug := steps[1].Env
	assert.Equal(t, "chat-secret-for-notify", plug["DHOW_INPUT_TOKEN"])
	assert.Equal(t, "#ops", plug["DHOW_INPUT_CHANNEL"])
	assert.Equal(t, sandbox.OutputPath, plug["DHOW_OUTPUT"])
	for name, v := range plug {
		assert.NotContains(t, v, "job-wide-deploy-secret", name)
	}
	for _, name := range []string{"DEPLOY_TOKEN", "JOB_SETTING", "DHOW_FENCING_TOKEN", "DHOW_JOB_ID", "DHOW_ATTEMPT_ID", "DHOW_STEP_INDEX"} {
		assert.NotContains(t, plug, name)
	}
}

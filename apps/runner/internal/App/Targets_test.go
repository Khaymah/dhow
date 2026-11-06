//go:build dhowtest

package app_test

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	runnertest "github.com/Khaymah/dhow/apps/runner/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/runner/internal/Config"
	executor "github.com/Khaymah/dhow/apps/runner/internal/Executor"
	sandbox "github.com/Khaymah/dhow/apps/runner/internal/Sandbox"
	fake "github.com/Khaymah/dhow/apps/runner/internal/Sandbox/Fake"
	"github.com/Khaymah/dhow/internal/jobspec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const (
	deployImage    = "ghcr.io/khaymah/dhow-deploy-ssh@sha256:3333333333333333333333333333333333333333333333333333333333333333"
	deployManifest = `{"Version":1,"Entrypoint":["/deploy"],"Mutating":true,"Inputs":{"target":{"Required":true},"artifact":{"Required":true,"Kind":"artifact"}},"ExitReasons":{"1":"DEPLOY_FAILED","10":"DEPLOY_ROLLED_BACK","11":"DEPLOY_HEALTH_FAILED","12":"DEPLOY_ROLLBACK_FAILED","13":"HOST_KEY_MISMATCH","15":"TARGET_LOCKED","18":"TARGET_STATE_CORRUPT"}}`
	releaseID      = "12-abcdef01-0"
	targetKeyLine1 = "b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW"
	targetKeyLine2 = "QyNTUxOQAAACD3d5eD0W6rQmJXm5nQ0m5rWQ7s5oX6lWQ0v2q2QJ0m3AAAAJgq1ZZ"
	targetKeyText  = "-----BEGIN OPENSSH " + "PRIVATE KEY-----\n" + targetKeyLine1 + "\n" + targetKeyLine2 + "\n-----END OPENSSH " + "PRIVATE KEY-----\n"
)

func targetKnownHost(host string, port int) string {
	blob := binary.BigEndian.AppendUint32(nil, uint32(len("ssh-ed25519")))
	blob = append(blob, "ssh-ed25519"...)
	blob = append(blob, "key-material"...)
	return jobspec.KnownHostsLine(host, port, "ssh-ed25519 "+base64.StdEncoding.EncodeToString(blob))
}

type targetJob struct {
	spec   *runnerv1.JobSpec
	digest string
	files  map[string]string
}

func newTargetJob(t *testing.T, srv *runnertest.Server, endpoints ...string) targetJob {
	t.Helper()
	files := map[string]string{"app/index.html": "<h1>hello</h1>"}
	digest := srv.PutBlobBytes(tarball(t, files))
	var hosts []*runnerv1.TargetHost
	for _, e := range endpoints {
		host, port, ok := jobspec.ParseEndpoint(e)
		require.True(t, ok, e)
		hosts = append(hosts, &runnerv1.TargetHost{HostId: uuid.NewString(), Host: host, Port: int32(port), KnownHostsLine: targetKnownHost(host, port)})
	}
	spec := runnertest.Spec(
		&runnerv1.Step{Name: "prepare", Run: "./prepare.sh"},
		&runnerv1.Step{Name: "deploy", Mutating: true, Plugin: &runnerv1.Plugin{Image: deployImage, Inputs: map[string]string{"target": "web", "artifact": "site.tgz"}}},
		&runnerv1.Step{Name: "notify", Plugin: &runnerv1.Plugin{Image: pluginImage, Inputs: map[string]string{"channel": "#ops"}}},
		&runnerv1.Step{Name: "after", Run: "./after.sh"},
	)
	spec.Inputs = []*runnerv1.ArtifactInput{{Name: "site.tgz", Digest: digest, SizeBytes: int64(len(srv.Blob(digest))), DownloadToken: runnertest.InputToken(digest)}}
	spec.Secrets = []*runnerv1.SecretRef{{
		Name: jobspec.TargetKeySecret(0), Target: &runnerv1.SecretRef_AsFile{AsFile: jobspec.TargetKeyFile(0)}, StepIndexes: []int32{1},
	}}
	spec.Network = &runnerv1.Network{ExactEndpoints: endpoints}
	spec.Targets = []*runnerv1.StepTarget{{
		StepIndex: 1, TargetId: uuid.NewString(), Version: 2, Name: "web", Kind: "ssh", User: "deploy", DeployPath: "/srv/web",
		ReleaseId: releaseID, KeepReleases: 5, Hosts: hosts, Settings: &runnerv1.TargetSettings{BatchSize: 1, Shared: []string{".env"}},
	}}
	return targetJob{spec: spec, digest: digest, files: files}
}

func targetImages() map[string]map[string]string {
	imgs := images()
	imgs[deployImage] = map[string]string{sandbox.PluginManifestLabel: deployManifest}
	return imgs
}

func targetRunner(t *testing.T, srv *runnertest.Server, cidrs string) *runnertest.Runner {
	t.Helper()
	r := checkoutRunner(t, srv)
	if cidrs != "" {
		r.Env[config.EnvEgressAllowPrivate] = cidrs
	}
	return r
}

func runTarget(t *testing.T, r *runnertest.Runner, srv *runnertest.Server, job targetJob, rec fake.Recording) (*runnertest.Running, executor.Outcome) {
	t.Helper()
	srv.Enqueue(job.spec, map[string][]byte{jobspec.TargetKeySecret(0): []byte(targetKeyText)})
	rec.Images = targetImages()
	running := r.Start(rec, runnertest.StartOptions{Workspace: t.TempDir()})
	return running, running.Next(t, finishWithin)
}

func stepExecs(events []fake.Event) map[int32]fake.Event {
	out := map[int32]fake.Event{}
	for _, e := range execs(events) {
		out[e.Step] = e
	}
	return out
}

func TestOnlyTheDeployStepSeesTheKeyFileTheTargetEnvironmentAndTheReport(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := targetRunner(t, srv, "")
	job := newTargetJob(t, srv, "web-1.example.com:22", "203.0.113.9:2222")
	running, out := runTarget(t, r, srv, job, fake.Recording{})
	require.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, out.Result, out.Reason.String())

	steps := stepExecs(running.Driver.Events())
	deploy := steps[1]
	assert.Equal(t, []string{"/deploy"}, deploy.Argv)
	assert.Equal(t, []string{"target-key"}, deploy.FileNames)
	assert.Equal(t, targetKeyText, string(deploy.Saw["target-key"]))
	assert.Equal(t, sandbox.SecretsPath+"/target-key", deploy.Env[jobspec.EnvTargetKeyFile])
	assert.Equal(t, "web", deploy.Env[jobspec.EnvTargetName])
	assert.Equal(t, "ssh", deploy.Env[jobspec.EnvTargetKind])
	assert.Equal(t, "deploy", deploy.Env[jobspec.EnvTargetUser])
	assert.Equal(t, "/srv/web", deploy.Env[jobspec.EnvTargetDeployPath])
	assert.Equal(t, releaseID, deploy.Env[jobspec.EnvTargetReleaseID])
	assert.Equal(t, sandbox.ReportPath, deploy.Env["DHOW_DEPLOY_REPORT"])
	var hosts []map[string]any
	require.NoError(t, json.Unmarshal([]byte(deploy.Env[jobspec.EnvTargetHosts]), &hosts))
	require.Len(t, hosts, 2)
	assert.Equal(t, "web-1.example.com", hosts[0]["Host"])

	for idx, e := range steps {
		if idx == 1 {
			continue
		}
		assert.NotContains(t, e.FileNames, "target-key", "step %d", idx)
		for name, v := range e.Env {
			assert.False(t, strings.HasPrefix(name, "DHOW_TARGET_"), "step %d got %s", idx, name)
			assert.NotEqual(t, "DHOW_DEPLOY_REPORT", name, "step %d", idx)
			assert.NotContains(t, v, targetKeyLine1, "step %d env %s", idx, name)
		}
		for _, f := range e.FileData {
			assert.NotContains(t, string(f), targetKeyLine1, "step %d", idx)
		}
		assert.NotContains(t, strings.Join(e.Argv, " "), targetKeyLine1)
	}
	for name, v := range deploy.Env {
		assert.NotContains(t, v, targetKeyLine1, "the key must stay in the file, not in %s", name)
	}
}

func TestEveryStepOfATargetJobRunsInASandboxOfItsOwnSoNoProcessOrFileIsShared(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := targetRunner(t, srv, "")
	job := newTargetJob(t, srv, "web-1.example.com:22")
	running, out := runTarget(t, r, srv, job, fake.Recording{})
	require.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, out.Result, out.Reason.String())

	var kinds []sandbox.Kind
	var images []string
	for _, e := range running.Driver.Events() {
		if e.Kind == fake.EventPrepare {
			kinds = append(kinds, e.Box)
			images = append(images, e.Image)
		}
	}
	assert.Equal(t, []sandbox.Kind{sandbox.KindTransfer, sandbox.KindJob, sandbox.KindPlugin, sandbox.KindPlugin, sandbox.KindJob}, kinds)
	assert.Equal(t, deployImage, images[2], "the deploy plugin has its own sandbox")
	assert.Equal(t, pluginImage, images[3], "another plugin of the same job has another one")
	l := srv.WaitLease(t, finishWithin)
	assert.True(t, l.MutatingSteps[1], "the manifest and the server both mark the deploy step as changing something")
	assert.False(t, l.MutatingSteps[0])
	assert.False(t, l.MutatingSteps[2])
}

func TestTheManifestAloneMarksAPluginStepAsMutating(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := targetRunner(t, srv, "")
	job := newTargetJob(t, srv, "web-1.example.com:22")
	job.spec.Steps[1].Mutating = true
	running, out := runTarget(t, r, srv, job, fake.Recording{})
	require.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, out.Result)
	_ = running
	assert.True(t, srv.WaitLease(t, finishWithin).MutatingSteps[1])

	srv2 := runnertest.NewServer(t)
	r2 := checkoutRunner(t, srv2)
	spec := runnertest.Spec(&runnerv1.Step{Name: "deploy", Plugin: &runnerv1.Plugin{Image: pluginImage, Inputs: map[string]string{"channel": "#ops"}}})
	srv2.Enqueue(spec, nil)
	rec := fake.Recording{Images: map[string]map[string]string{pluginImage: {
		sandbox.PluginManifestLabel: `{"Version":1,"Entrypoint":["/bin/x"],"Mutating":true,"Inputs":{"channel":{"Required":true}}}`,
	}}}
	running2 := r2.Start(rec, runnertest.StartOptions{})
	require.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, running2.Next(t, finishWithin).Result)
	assert.True(t, srv2.WaitLease(t, finishWithin).MutatingSteps[0], "a manifest that says Mutating is enough")
}

func TestTheKeyIsMaskedInTheLogEvenWhenThePluginPrintsIt(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := targetRunner(t, srv, "")
	job := newTargetJob(t, srv, "web-1.example.com:22")
	rec := fake.Recording{Steps: []fake.Step{{}, {Output: []fake.Output{
		{Stream: "stdout", Data: targetKeyText},
		{Stream: "stderr", Data: "line=" + targetKeyLine2 + "\n"},
		{Stream: "stdout", Data: base64.StdEncoding.EncodeToString([]byte(targetKeyText)) + "\n"},
	}}}}
	_, out := runTarget(t, r, srv, job, rec)
	require.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, out.Result, out.Reason.String())
	var log strings.Builder
	for _, c := range srv.WaitLease(t, finishWithin).Chunks {
		log.Write(c.GetData())
	}
	assert.NotEmpty(t, log.String())
	for _, part := range []string{targetKeyLine1, targetKeyLine2, base64.StdEncoding.EncodeToString([]byte(targetKeyText))[:40]} {
		assert.NotContains(t, log.String(), part)
	}
	assert.Contains(t, log.String(), "***")
}

func TestTheArchiveGoesToTheDeployStepAsAFileAndIsNeverUnpacked(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := targetRunner(t, srv, "")
	job := newTargetJob(t, srv, "web-1.example.com:22")
	running, out := runTarget(t, r, srv, job, fake.Recording{})
	require.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, out.Result, out.Reason.String())

	deploy := stepExecs(running.Driver.Events())[1]
	assert.Equal(t, sandbox.InputsPath+"/site.tgz", deploy.Env["DHOW_INPUT_ARTIFACT_FILE"])
	assert.Equal(t, job.digest, deploy.Env["DHOW_INPUT_ARTIFACT_DIGEST"])
	assert.Equal(t, "site.tgz", deploy.Env["DHOW_INPUT_ARTIFACT"])
	assert.Empty(t, transferExecs(running.Driver.Events()), "no unpack ran, so the archive was not extracted anywhere")

	var withInputs int
	for _, e := range running.Driver.Events() {
		if e.Kind != fake.EventPrepare || len(e.Inputs) == 0 {
			continue
		}
		withInputs++
		assert.Equal(t, sandbox.KindPlugin, e.Box)
		assert.Equal(t, deployImage, e.Image)
		assert.Equal(t, srv.Blob(job.digest), e.Inputs["site.tgz"], "the bytes are the archive the digest names")
	}
	assert.Equal(t, 1, withInputs, "only the deploy step's sandbox got the file")
	for idx, e := range stepExecs(running.Driver.Events()) {
		if idx != 1 {
			assert.NotContains(t, e.Env, "DHOW_INPUT_ARTIFACT_FILE", "step %d", idx)
		}
	}
}

func TestAnArtifactThatWasNotDeliveredFailsTheDeployStepBeforeItStarts(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := targetRunner(t, srv, "")
	job := newTargetJob(t, srv, "web-1.example.com:22")
	job.spec.Steps[1].Plugin.Inputs["artifact"] = "other.tgz"
	running, out := runTarget(t, r, srv, job, fake.Recording{})
	assert.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_FAILED, out.Result)
	assert.Equal(t, runnerv1.FailureReason_FAILURE_REASON_PLUGIN_INPUT_INVALID, out.Reason)
	_, ran := stepExecs(running.Driver.Events())[1]
	assert.False(t, ran, "the plugin never started, so it never saw the key")
}

func TestADigestThatDoesNotMatchStopsTheJobBeforeTheDeployStep(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := targetRunner(t, srv, "")
	job := newTargetJob(t, srv, "web-1.example.com:22")
	srv.ConfigureBlobs(func(k *runnertest.BlobKnobs) { k.CorruptDownload = true })
	running, out := runTarget(t, r, srv, job, fake.Recording{})
	assert.Equal(t, runnerv1.FailureReason_FAILURE_REASON_ARTIFACT_DOWNLOAD_FAILED, out.Reason)
	assert.Empty(t, execs(running.Driver.Events()))
}

type tunnelResult struct {
	status int
	echo   bool
}

func tunnel(t *testing.T, proxy, target string) tunnelResult {
	t.Helper()
	c, err := net.Dial("tcp", proxy)
	require.NoError(t, err)
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(10 * time.Second))
	_, err = io.WriteString(c, "CONNECT "+target+" HTTP/1.1\r\nHost: "+target+"\r\n\r\n")
	require.NoError(t, err)
	br := bufio.NewReader(c)
	resp, err := http.ReadResponse(br, nil)
	require.NoError(t, err)
	_ = resp.Body.Close()
	res := tunnelResult{status: resp.StatusCode}
	if resp.StatusCode == http.StatusOK {
		_, err = io.WriteString(c, "ping")
		require.NoError(t, err)
		got := make([]byte, 4)
		_, err = io.ReadFull(br, got)
		res.echo = err == nil && bytes.Equal(got, []byte("ping"))
	}
	return res
}

func echoListener(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = c.Close() }()
				_, _ = io.Copy(c, c)
			}()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

func deployProxy(t *testing.T, running *runnertest.Running) string {
	t.Helper()
	var addr string
	runnertest.Eventually(t, finishWithin, func() bool {
		for _, s := range running.Driver.Specs() {
			if s.Kind == sandbox.KindPlugin && len(s.Exact) > 0 {
				addr = running.Driver.EgressAddr(s.Seq)
			}
		}
		return addr != ""
	}, "the egress proxy of the deploy sandbox")
	return addr
}

func TestTheDeploySandboxReachesTheTargetEndpointAndNothingElse(t *testing.T) {
	srv := runnertest.NewServer(t)
	port := echoListener(t)
	r := targetRunner(t, srv, "127.0.0.1/32")
	endpoint := fmt.Sprintf("127.0.0.1:%d", port)
	job := newTargetJob(t, srv, endpoint)
	srv.Enqueue(job.spec, map[string][]byte{jobspec.TargetKeySecret(0): []byte(targetKeyText)})
	running := r.Start(fake.Recording{Images: targetImages(), Steps: []fake.Step{{}, {HangMillis: 3000}}}, runnertest.StartOptions{Workspace: t.TempDir()})
	proxy := deployProxy(t, running)

	assert.Equal(t, tunnelResult{status: http.StatusOK, echo: true}, tunnel(t, proxy, endpoint))
	for _, other := range []string{
		fmt.Sprintf("127.0.0.1:%d", port+1), "registry.npmjs.org:443", "github.com:443", "198.51.100.7:22", "[::1]:" + fmt.Sprint(port), "localhost:" + fmt.Sprint(port),
	} {
		assert.Equal(t, http.StatusForbidden, tunnel(t, proxy, other).status, other)
	}
	require.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, running.Next(t, finishWithin).Result)
	deploy := stepExecs(running.Driver.Events())[1]
	for _, name := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "ALL_PROXY"} {
		assert.Equal(t, executor.ProxyURL, deploy.Env[name], "a job with only exact endpoints still gets the proxy variables: %s", name)
	}
	assert.Equal(t, executor.ProxyURL, stepExecs(running.Driver.Events())[0].Env["HTTPS_PROXY"])
}

func TestAPrivateEndpointIsRefusedWithoutTheOperatorRange(t *testing.T) {
	srv := runnertest.NewServer(t)
	port := echoListener(t)
	r := targetRunner(t, srv, "")
	endpoint := fmt.Sprintf("127.0.0.1:%d", port)
	job := newTargetJob(t, srv, endpoint)
	srv.Enqueue(job.spec, map[string][]byte{jobspec.TargetKeySecret(0): []byte(targetKeyText)})
	running := r.Start(fake.Recording{Images: targetImages(), Steps: []fake.Step{{}, {HangMillis: 3000}}}, runnertest.StartOptions{Workspace: t.TempDir()})
	proxy := deployProxy(t, running)
	assert.Equal(t, http.StatusForbidden, tunnel(t, proxy, endpoint).status, "the endpoint is listed, but a loopback address is not the operator's to open by default")
	require.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, running.Next(t, finishWithin).Result)
}

func reportLines(lines ...string) []fake.Output {
	out := make([]fake.Output, 0, len(lines))
	for _, l := range lines {
		out = append(out, fake.Output{Stream: "report", Data: l + "\n"})
	}
	return out
}

func TestTheAgentReportsReachTheServerInOrderMaskedAndWithinTheLimits(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := targetRunner(t, srv, "")
	job := newTargetJob(t, srv, "web-1.example.com:22")
	host := job.spec.Targets[0].Hosts[0].HostId
	lines := reportLines(
		`{"Event":"release_started","ReleaseId":"`+releaseID+`","PreviousReleaseId":"11-aaaaaaaa-0","Mode":"deploy"}`,
		`{"Event":"uploaded","HostId":"`+host+`","Batch":1}`,
		`not json at all`,
		`{"Event":"unknown_kind"}`,
		`{"Event":"switched","HostId":"`+host+`","Batch":1,"Extra":"field"}`,
		`{"Event":"health","HostId":"`+host+`","Message":"checked with `+targetKeyLine2+`","Health":{"Attempts":3,"Passed":true,"LastStatus":200,"DurationMs":1200,"StableChecks":3}}`,
		`{"Event":"verified","HostId":"`+host+`"}`,
		`{"Event":"verified"}`,
	)
	running, out := runTarget(t, r, srv, job, fake.Recording{Steps: []fake.Step{{}, {Output: lines}}})
	require.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, out.Result, out.Reason.String())
	_ = running

	got := srv.ReportedEvents()
	var kinds []runnerv1.DeployEventKind
	for _, e := range got {
		kinds = append(kinds, e.GetKind())
	}
	assert.Equal(t, []runnerv1.DeployEventKind{
		runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_RELEASE_STARTED, runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_UPLOADED,
		runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_HEALTH, runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_VERIFIED, runnerv1.DeployEventKind_DEPLOY_EVENT_KIND_VERIFIED,
	}, kinds, "invalid lines, unknown kinds and lines with unknown fields are dropped")
	assert.Equal(t, releaseID, got[0].GetReleaseId())
	assert.Equal(t, host, got[1].GetHostId())
	assert.NotContains(t, got[2].GetMessage(), targetKeyLine2, "the runner masks the message with the job's secrets")
	assert.Equal(t, int32(3), got[2].GetHealth().GetAttempts())
	for _, c := range srv.ReportCalls() {
		assert.Equal(t, int32(1), c.Step)
		assert.Equal(t, srv.Leases()[0].ID, c.LeaseID)
		assert.Equal(t, srv.Leases()[0].Fencing, c.Fencing)
	}
}

func TestOnlyTheFirstHundredReportEventsAreSent(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := targetRunner(t, srv, "")
	job := newTargetJob(t, srv, "web-1.example.com:22")
	host := job.spec.Targets[0].Hosts[0].HostId
	var lines []string
	for range 130 {
		lines = append(lines, `{"Event":"health","HostId":"`+host+`"}`)
	}
	_, out := runTarget(t, r, srv, job, fake.Recording{Steps: []fake.Step{{}, {Output: reportLines(lines...)}}})
	require.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, out.Result)
	assert.Len(t, srv.ReportedEvents(), executor.MaxReportEvents)
}

func TestAReportThatTheServerCannotTakeYetIsRetriedAndNeverFailsTheJob(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := targetRunner(t, srv, "")
	job := newTargetJob(t, srv, "web-1.example.com:22")
	srv.FailReports(2)
	_, out := runTarget(t, r, srv, job, fake.Recording{Steps: []fake.Step{{}, {Output: reportLines(`{"Event":"release_started","ReleaseId":"` + releaseID + `"}`)}}})
	require.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, out.Result)
	assert.Len(t, srv.ReportedEvents(), 1)
}

func TestAStepOfAJobWithoutATargetWritesNoReport(t *testing.T) {
	srv := runnertest.NewServer(t)
	r := checkoutRunner(t, srv)
	spec := runnertest.Spec(&runnerv1.Step{Name: "notify", Plugin: &runnerv1.Plugin{Image: pluginImage, Inputs: map[string]string{"channel": "#ops"}}})
	srv.Enqueue(spec, nil)
	running := r.Start(fake.Recording{Images: images(), Steps: []fake.Step{{Output: reportLines(`{"Event":"verified"}`)}}}, runnertest.StartOptions{})
	require.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, running.Next(t, finishWithin).Result)
	assert.Empty(t, srv.ReportedEvents())
	assert.NotContains(t, stepExecs(running.Driver.Events())[0].Env, "DHOW_DEPLOY_REPORT")
}

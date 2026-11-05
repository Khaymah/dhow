//go:build dhowtest

package app_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	runnertest "github.com/Khaymah/dhow/apps/runner/internal/App/Testing"
	sandbox "github.com/Khaymah/dhow/apps/runner/internal/Sandbox"
	fake "github.com/Khaymah/dhow/apps/runner/internal/Sandbox/Fake"
	"github.com/Khaymah/dhow/internal/jobspec"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

func twoTargetJob(t *testing.T, srv *runnertest.Server, secondArtifact string) targetJob {
	t.Helper()
	job := newTargetJob(t, srv, "web-1.example.com:22", "api-1.example.com:22")
	other := srv.PutBlobBytes(tarball(t, map[string]string{"api/index.html": "<h1>api</h1>"}))
	if secondArtifact == "site.tgz" {
		other = job.digest
	} else {
		job.spec.Inputs = append(job.spec.Inputs, &runnerv1.ArtifactInput{
			Name: secondArtifact, Digest: other, SizeBytes: int64(len(srv.Blob(other))), DownloadToken: runnertest.InputToken(other),
		})
	}
	job.spec.Network.ExactEndpoints = []string{"web-1.example.com:22", "api-1.example.com:22"}
	job.spec.Steps = append(job.spec.Steps, &runnerv1.Step{
		Name: "deploy-api", Mutating: true,
		Plugin: &runnerv1.Plugin{Image: deployImage, Inputs: map[string]string{"target": "api", "artifact": secondArtifact}},
	})
	job.spec.Secrets = append(job.spec.Secrets, &runnerv1.SecretRef{
		Name: jobspec.TargetKeySecret(1), Target: &runnerv1.SecretRef_AsFile{AsFile: jobspec.TargetKeyFile(1)}, StepIndexes: []int32{4},
	})
	job.spec.Targets[0].Hosts = job.spec.Targets[0].Hosts[:1]
	job.spec.Targets = append(job.spec.Targets, &runnerv1.StepTarget{
		StepIndex: 4, TargetId: uuid.NewString(), Version: 1, Name: "api", Kind: "ssh", User: "deploy", DeployPath: "/srv/api",
		ReleaseId: "12-abcdef01-1", KeepReleases: 5, Settings: &runnerv1.TargetSettings{BatchSize: 1},
		Hosts: []*runnerv1.TargetHost{{HostId: uuid.NewString(), Host: "api-1.example.com", Port: 22, KnownHostsLine: targetKnownHost("api-1.example.com", 22)}},
	})
	job.digest = other
	return job
}

func TestTwoDeployStepsOfOneJobEachGetTheirOwnArchiveAndKey(t *testing.T) {
	for _, second := range []string{"api.tgz", "site.tgz"} {
		t.Run(second, func(t *testing.T) {
			srv := runnertest.NewServer(t)
			r := targetRunner(t, srv, "")
			job := twoTargetJob(t, srv, second)
			srv.Enqueue(job.spec, map[string][]byte{
				jobspec.TargetKeySecret(0): []byte(targetKeyText), jobspec.TargetKeySecret(1): []byte(targetKeyText + "second"),
			})
			running := r.Start(fake.Recording{Images: targetImages()}, runnertest.StartOptions{Workspace: t.TempDir()})
			out := running.Next(t, finishWithin)
			require.Equal(t, runnerv1.AttemptResult_ATTEMPT_RESULT_SUCCEEDED, out.Result, out.Reason.String())

			steps := stepExecs(running.Driver.Events())
			assert.Equal(t, targetKeyText, string(steps[1].Saw["target-key"]))
			assert.Equal(t, targetKeyText+"second", string(steps[4].Saw["target-key-1"]))
			assert.NotContains(t, steps[1].FileNames, "target-key-1")
			assert.NotContains(t, steps[4].FileNames, "target-key")
			assert.Equal(t, sandbox.InputsPath+"/"+second, steps[4].Env["DHOW_INPUT_ARTIFACT_FILE"])
			var withInputs int
			for _, e := range running.Driver.Events() {
				if e.Kind == fake.EventPrepare && len(e.Inputs) > 0 {
					withInputs++
				}
			}
			assert.Equal(t, 2, withInputs, "both deploy sandboxes got their archive")
		})
	}
}

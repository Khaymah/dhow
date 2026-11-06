package e2e_test

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	"github.com/Khaymah/dhow/internal/jobspec"
)

const (
	sshPlugin      = "ghcr.io/khaymah/dhow-deploy-ssh@sha256:03fddb3b486b758f338f8999afbf04ced985d74825963dd28b92e650645e6778"
	thirdParty     = "ghcr.io/acme/deploy@sha256:9b0fdebd63cc544675f3d2a9b00f975ebc573d95b42602523ea996b62c3b6a68"
	keySecretName  = "WEB_SSH_KEY"
	targetKeyValue = "the-private-half-of-a-deploy-key-0123456789"
	hostOne        = "web-1.example.com"
	hostTwo        = "web-2.example.com"
	hostThree      = "web-3.example.com"

	deployPipeline = `version: 1
on:
  push: {}
jobs:
  build:
    image: IMAGE
    steps:
      - run: make
    artifacts:
      - name: web
        paths: [dist/]
  deploy:
    image: IMAGE
    needs: [build]
    environment: production
    steps:
      - run: ./prepare.sh
      - uses: PLUGIN
        with:
          artifact: web
          target: web
          health_check_url: https://{host}/healthz
`
)

type deployWorld struct {
	world
	runner *apptest.TestRunner
	prod   uuid.UUID
	target apptest.SeededTarget
}

func newDeployWorld(t *testing.T) deployWorld {
	t.Helper()
	w := newWorldWith(t, apptest.Options{DeploySSHImage: sshPlugin})
	envID := w.env.SeedEnvironment(w.project, "production")
	w.env.SeedSecret(apptest.SecretRef(w.org, &w.project, "production"), keySecretName, targetKeyValue)
	w.env.SeedVariable(apptest.SecretRef(w.org, &w.project, "production"), "TARGET", "web")
	target := w.env.SeedPinnedTarget(w.project, "production", "web", keySecretName, []string{hostOne, hostTwo, hostThree})
	runner := w.env.SeedConnectedRunner(w.org, w.ownerID, apptest.SeedRunnerSpec{
		Capacity: 4, Features: []string{"checkout", "plugins", "deploy_targets"},
	})
	builder := w.env.SeedConnectedRunner(w.org, w.ownerID, apptest.SeedRunnerSpec{
		Capacity: 4, Features: []string{"checkout", "plugins", "artifacts"},
	})
	w.runners = append(w.runners, builder)
	return deployWorld{world: w, runner: runner, prod: envID, target: target}
}

func (d deployWorld) start(yaml string) uuid.UUID {
	d.env.T.Helper()
	d.defs.Put(shaA, strings.ReplaceAll(pipeline(yaml), "PLUGIN", sshPlugin))
	run := d.push("refs/heads/main", shaA)
	d.env.StartRuns()
	return run
}

func (d deployWorld) built(t *testing.T, yaml string) uuid.UUID {
	t.Helper()
	run := d.start(yaml)
	d.complete(t, "build")
	return run
}

func (d deployWorld) sql(t *testing.T, query string, args ...any) {
	t.Helper()
	d.exec(t, query, args...)
}

func (d deployWorld) keyUsed(t *testing.T) bool {
	t.Helper()
	var used *string
	require.NoError(t, d.env.Pool.QueryRow(context.Background(),
		`SELECT "LastUsedAt"::text FROM "Secrets" WHERE "Name" = $1`, keySecretName).Scan(&used))
	return used != nil
}

func (d deployWorld) leaseDeploy(t *testing.T) *apptest.Grant {
	t.Helper()
	g, err := d.runner.Lease(context.Background(), 1, 0)
	require.NoError(t, err)
	require.NotNil(t, g, "the deploy runner got the job")
	require.Equal(t, "deploy", d.env.RunJobKey(uuid.MustParse(g.Grant.GetJobId())))
	return g
}

func (d deployWorld) requireNoKeyOpened(t *testing.T) {
	t.Helper()
	assert.False(t, d.keyUsed(t), "the key of the target was never opened")
	assert.Equal(t, 0, d.env.AuditCount("job.secrets_delivered"), "nothing was delivered, so nothing was audited")
}

func TestADeployJobCarriesTheTargetOfItsEnvironment(t *testing.T) {
	d := newDeployWorld(t)
	run := d.built(t, deployPipeline)
	require.Equal(t, "active", d.env.RunJobs(run)["deploy"].State)

	for _, r := range d.runners[:3] {
		g, err := r.Lease(context.Background(), 1, 0)
		require.NoError(t, err)
		assert.Nil(t, g, "a runner without deploy_targets never gets a job with a target")
	}
	g := d.leaseDeploy(t)
	spec := g.Grant.GetSpec()
	require.Len(t, spec.GetTargets(), 1)
	target := spec.GetTargets()[0]
	assert.Equal(t, int32(1), target.GetStepIndex(), "the plugin is the second step")
	assert.Equal(t, d.target.ID.String(), target.GetTargetId())
	assert.Equal(t, []string{"web", "ssh", "deploy", "/srv/web"}, []string{target.GetName(), target.GetKind(), target.GetUser(), target.GetDeployPath()})
	assert.Regexp(t, regexp.MustCompile(`^[0-9]+-11111111-[0-9]+$`), target.GetReleaseId())
	assert.Equal(t, int32(5), target.GetKeepReleases())
	require.Len(t, target.GetHosts(), 3)
	for i, h := range target.GetHosts() {
		want := d.target.Hosts[i]
		assert.Equal(t, []string{want.ID.String(), want.Host, "ssh-ed25519"}, []string{h.GetHostId(), h.GetHost(), strings.Fields(h.GetKnownHostsLine())[1]}, "host %d keeps its position", i)
		assert.Equal(t, int32(22), h.GetPort())
		assert.Equal(t, want.Host+" "+want.HostKey, h.GetKnownHostsLine(), "each host is pinned to its own key")
	}
	st := target.GetSettings()
	assert.Equal(t, []int32{2, 30}, []int32{st.GetBatchSize(), st.GetBatchPauseSeconds()})
	assert.Equal(t, "sudo systemctl reload web", st.GetReloadCommand())
	assert.Equal(t, []string{".env", "storage"}, st.GetShared())
	assert.True(t, spec.GetSteps()[1].GetMutating(), "the deploy step is a mutating step, so a retry stops after it starts")
	assert.False(t, spec.GetSteps()[0].GetMutating())
	assert.Empty(t, spec.GetNetwork().GetEgress())
	assert.Equal(t, []string{
		hostOne + ":22", hostTwo + ":22", hostThree + ":22", hostOne + ":443", hostTwo + ":443", hostThree + ":443",
	}, spec.GetNetwork().GetExactEndpoints(), "the hosts, then the health check on every host")
	require.NoError(t, jobspec.Validate(spec), "the runner accepts the spec the server made")
	assert.Contains(t, jobspec.Features(spec), jobspec.FeatureDeployTargets)

	refs := spec.GetSecrets()
	require.Len(t, refs, 1, "the target key is the only secret of the job")
	assert.Equal(t, "dhow.target.0.key", refs[0].GetName())
	assert.Equal(t, "target-key", refs[0].GetAsFile())
	assert.Equal(t, []int32{1}, refs[0].GetStepIndexes(), "only the plugin step gets the key, not the run step")

	start, err := d.runner.Start(g)
	require.NoError(t, err)
	values := openStart(t, d.runner, g, start)
	assert.Equal(t, map[string]string{"dhow.target.0.key": targetKeyValue}, values)
	assert.True(t, d.keyUsed(t))
	var data string
	require.NoError(t, d.env.Pool.QueryRow(context.Background(),
		`SELECT "Data"::text FROM "AuditEvents" WHERE "Action" = 'job.secrets_delivered'`).Scan(&data))
	assert.Contains(t, data, keySecretName)
	assert.NotContains(t, data, "private-half")
}

func TestAnExpressionIsNotATargetName(t *testing.T) {
	d := newDeployWorld(t)
	run := d.built(t, strings.Replace(deployPipeline, "target: web", "target: ${{ vars.TARGET }}", 1))
	deploy := d.env.RunJobs(run)["deploy"]
	assert.Equal(t, []string{"failed", "TARGET_NOT_LITERAL"}, []string{deploy.State, deploy.Reason})
	d.requireNoKeyOpened(t)
}

func TestEveryTargetRefusalFailsTheJobBeforeTheKeyIsOpened(t *testing.T) {
	longHealth := strings.Replace(deployPipeline, "health_check_url: https://{host}/healthz",
		"health_check_url: https://{host}/healthz\n          health_check_retries: 60\n          health_check_interval: 30s\n          health_check_timeout: 30s", 1)
	cases := []struct {
		name   string
		yaml   string
		change string
		reason string
		names  string
	}{
		{"a target of another environment", deployPipeline, `UPDATE "DeployTargets" SET "EnvironmentId" = (
			SELECT e."Id" FROM "Environments" e WHERE e."Name" = 'staging') WHERE "Name" = 'web'`, "TARGET_NOT_FOUND", "web"},
		{"a target that does not exist", strings.Replace(deployPipeline, "target: web", "target: nope", 1), "", "TARGET_NOT_FOUND", "nope"},
		{"a job without an environment", strings.Replace(deployPipeline, "    environment: production\n", "", 1), "", "TARGET_REQUIRES_ENVIRONMENT", ""},
		{"a host without a pinned key", deployPipeline, `UPDATE "DeployTargetHosts" SET "HostKey" = NULL, "HostKeyFingerprint" = NULL,
			"HostKeyConfirmedAt" = NULL, "HostKeyConfirmedBy" = NULL WHERE "Host" = 'web-2.example.com'`, "TARGET_HOST_KEY_UNCONFIRMED", hostTwo + ":22"},
		{"a key secret that is gone", deployPipeline, `UPDATE "DeployTargets" SET "KeySecretName" = 'GONE_KEY' WHERE "Name" = 'web'`, "TARGET_KEY_NOT_FOUND", "GONE_KEY"},
		{"a key secret of the project scope", deployPipeline, `UPDATE "Secrets" SET "EnvironmentId" = NULL, "Scope" = 'project' WHERE "Name" = 'WEB_SSH_KEY'`, "TARGET_KEY_NOT_FOUND", keySecretName},
		{"a plugin that is not official", strings.ReplaceAll(deployPipeline, "uses: PLUGIN", "uses: "+thirdParty), "", "TARGET_PLUGIN_NOT_ALLOWED", "web"},
		{"an artifact no job uploads", strings.Replace(deployPipeline, "artifact: web", "artifact: elsewhere", 1), "", "TARGET_ARTIFACT_REQUIRED", ""},
		{"no artifact at all", strings.Replace(deployPipeline, "          artifact: web\n", "", 1), "", "TARGET_ARTIFACT_REQUIRED", ""},
		{"more network than the target", strings.Replace(deployPipeline, "    needs: [build]\n", "    needs: [build]\n    network:\n      egress: [hooks.slack.com]\n", 1), "", "TARGET_JOB_EGRESS_NOT_ALLOWED", ""},
		{"a health address that is not http", strings.Replace(deployPipeline, "https://{host}/healthz", "ftp://{host}/healthz", 1), "", "HEALTH_URL_INVALID", ""},
		{"a health address with a user name", strings.Replace(deployPipeline, "https://{host}/healthz", "https://root@{host}/healthz", 1), "", "HEALTH_URL_INVALID", ""},
		{"a health address with the host inside the path only", strings.Replace(deployPipeline, "https://{host}/healthz", "https://example.com/{host}", 1), "", "HEALTH_URL_INVALID", ""},
		{"a health check on the metadata address", strings.Replace(deployPipeline, "https://{host}/healthz", "http://169.254.169.254/", 1), "", "HEALTH_URL_NOT_ALLOWED", ""},
		{"a health check on a private address", strings.Replace(deployPipeline, "https://{host}/healthz", "http://10.0.0.5/healthz", 1), "", "HEALTH_URL_NOT_ALLOWED", ""},
		{"a health check on a public host that is not the target", strings.Replace(deployPipeline, "https://{host}/healthz", "https://example.org/healthz", 1), "", "HEALTH_URL_NOT_ALLOWED", ""},
		{"a plugin whose name only starts with the official one", strings.ReplaceAll(deployPipeline, "uses: PLUGIN", "uses: ghcr.io/khaymah/dhow-deploy-ssh/evil@sha256:"+strings.Repeat("a", 64)), "", "TARGET_PLUGIN_NOT_ALLOWED", "web"},
		{"a run step that names the key of the target", strings.Replace(deployPipeline, "      - run: ./prepare.sh\n", "      - run: ./prepare.sh\n        env:\n          KEY: ${{ secrets.WEB_SSH_KEY }}\n", 1), "", "TARGET_KEY_NOT_REFERENCEABLE", keySecretName},
		{"a health check that runs longer than 10 minutes", longHealth, "", "HEALTH_BUDGET_EXCEEDED", ""},
		{
			"a rolling deploy that runs longer than 60 minutes", strings.Replace(deployPipeline, "https://{host}/healthz",
				"https://{host}/healthz\n          health_check_retries: 60\n          health_check_interval: 5s\n          health_check_timeout: 5s", 1),
			`UPDATE "DeployTargets" SET "Settings" = '{"Rolling":{"BatchSize":1,"BatchPauseSeconds":600},"Ssh":{"Shared":[]}}'::jsonb WHERE "Name" = 'web'`,
			"ROLLING_BUDGET_EXCEEDED", "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := newDeployWorld(t)
			d.env.SeedEnvironment(d.project, "staging")
			if strings.Contains(c.name, "rolling") {
				d.sql(t, `INSERT INTO "DeployTargetHosts" ("OrganizationId", "TargetId", "Position", "Host", "Port", "DeployPath", "HostKey", "HostKeyFingerprint",
					"HostKeyConfirmedAt", "HostKeyConfirmedBy")
					SELECT h."OrganizationId", h."TargetId", 4, 'web-4.example.com', 22, h."DeployPath", h."HostKey", h."HostKeyFingerprint", h."HostKeyConfirmedAt",
					h."HostKeyConfirmedBy" FROM "DeployTargetHosts" h WHERE h."Host" = 'web-1.example.com'`)
			}
			run := d.start(c.yaml)
			if c.change != "" {
				d.sql(t, c.change)
			}
			d.complete(t, "build")
			deploy := d.env.RunJobs(run)["deploy"]
			assert.Equal(t, []string{"failed", c.reason}, []string{deploy.State, deploy.Reason})
			if c.names != "" {
				assert.Contains(t, failureNames(t, d.world, run.String(), "deploy"), c.names)
			}
			g, err := d.runner.Lease(context.Background(), 1, 0)
			require.NoError(t, err)
			assert.Nil(t, g, "a refused job is never queued")
			d.requireNoKeyOpened(t)
		})
	}
}

func TestAThirdPartyPluginReceivesATargetOnlyWhenTheTargetAllowsItsImage(t *testing.T) {
	d := newDeployWorld(t)
	d.sql(t, `UPDATE "DeployTargets" SET "AllowedImages" = ARRAY['ghcr.io/acme/'] WHERE "Name" = 'web'`)
	run := d.built(t, strings.ReplaceAll(deployPipeline, "uses: PLUGIN", "uses: "+thirdParty))
	assert.Equal(t, "active", d.env.RunJobs(run)["deploy"].State)
	g := d.leaseDeploy(t)
	require.Len(t, g.Grant.GetSpec().GetTargets(), 1)
}

func TestExtraEgressIsAllowedOnlyWhenTheEnvironmentOpensIt(t *testing.T) {
	d := newDeployWorld(t)
	d.sql(t, `UPDATE "Environments" SET "AllowExtraDeployEgress" = true, "RuleVersion" = "RuleVersion" + 1 WHERE "Name" = 'production'`)
	run := d.built(t, strings.Replace(deployPipeline, "    needs: [build]\n", "    needs: [build]\n    network:\n      egress: [hooks.slack.com]\n", 1))
	assert.Equal(t, "active", d.env.RunJobs(run)["deploy"].State)
	spec := d.leaseDeploy(t).Grant.GetSpec()
	assert.Equal(t, []string{"hooks.slack.com"}, spec.GetNetwork().GetEgress(), "the egress of the file stays")
	assert.Contains(t, spec.GetNetwork().GetExactEndpoints(), hostOne+":22", "and the hosts of the target are added")
}

func TestATargetWithAnIPAddressGetsAnExactEndpoint(t *testing.T) {
	d := newDeployWorld(t)
	d.sql(t, `UPDATE "DeployTargetHosts" SET "Host" = '203.0.113.7' WHERE "Host" = 'web-1.example.com'`)
	d.sql(t, `UPDATE "DeployTargetHosts" SET "Port" = 2222 WHERE "Host" = 'web-2.example.com'`)
	run := d.built(t, strings.Replace(deployPipeline, "https://{host}/healthz", "https://"+hostThree+"/healthz", 1))
	require.Equal(t, "active", d.env.RunJobs(run)["deploy"].State)
	spec := d.leaseDeploy(t).Grant.GetSpec()
	assert.Equal(t, []string{"203.0.113.7:22", hostTwo + ":2222", hostThree + ":22", hostThree + ":443"}, spec.GetNetwork().GetExactEndpoints())
	assert.Empty(t, spec.GetNetwork().GetEgress(), "an IP address never goes into the egress list")
	assert.Equal(t, "["+hostTwo+"]:2222 "+d.target.Hosts[1].HostKey, spec.GetTargets()[0].GetHosts()[1].GetKnownHostsLine())
}

func TestWhatChangesAfterTheJobWasQueuedStopsItAtStartWithoutOpeningTheKey(t *testing.T) {
	cases := []struct {
		name   string
		change string
		reason string
	}{
		{"the version of the target", `UPDATE "DeployTargets" SET "Version" = "Version" + 1 WHERE "Name" = 'web'`, "TARGET_CHANGED"},
		{"the host key of one host", `UPDATE "DeployTargetHosts" SET "HostKey" = "HostKey" || 'x' WHERE "Host" = 'web-3.example.com'`, "TARGET_CHANGED"},
		{"the address of one host", `UPDATE "DeployTargetHosts" SET "Host" = 'web-9.example.com' WHERE "Host" = 'web-3.example.com'`, "TARGET_CHANGED"},
		{"the port of one host", `UPDATE "DeployTargetHosts" SET "Port" = 2222 WHERE "Host" = 'web-1.example.com'`, "TARGET_CHANGED"},
		{"a host removed", `UPDATE "DeployTargetHosts" SET "IsDeleted" = true WHERE "Host" = 'web-3.example.com'`, "TARGET_CHANGED"},
		{"a host added", `INSERT INTO "DeployTargetHosts" ("OrganizationId", "TargetId", "Position", "Host", "Port", "DeployPath", "HostKey", "HostKeyFingerprint",
			"HostKeyConfirmedAt", "HostKeyConfirmedBy") SELECT h."OrganizationId", h."TargetId", 4, 'web-4.example.com', 22, h."DeployPath", h."HostKey",
			h."HostKeyFingerprint", h."HostKeyConfirmedAt", h."HostKeyConfirmedBy" FROM "DeployTargetHosts" h WHERE h."Host" = 'web-1.example.com'`, "TARGET_CHANGED"},
		{"the order of the hosts", `UPDATE "DeployTargetHosts" SET "Position" = 101 WHERE "Host" = 'web-1.example.com';
			UPDATE "DeployTargetHosts" SET "Position" = 1 WHERE "Host" = 'web-2.example.com';
			UPDATE "DeployTargetHosts" SET "Position" = 2 WHERE "Host" = 'web-1.example.com'`, "TARGET_CHANGED"},
		{"the target removed", `UPDATE "DeployTargets" SET "IsDeleted" = true WHERE "Name" = 'web'`, "TARGET_CHANGED"},
		{"the key secret removed", `UPDATE "Secrets" SET "IsDeleted" = true WHERE "Name" = 'WEB_SSH_KEY'`, "TARGET_KEY_NOT_FOUND"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := newDeployWorld(t)
			run := d.built(t, deployPipeline)
			require.Equal(t, "active", d.env.RunJobs(run)["deploy"].State)
			g := d.leaseDeploy(t)
			d.sql(t, c.change)

			_, err := d.runner.Start(g)
			var ce *connect.Error
			require.ErrorAs(t, err, &ce, "Start is refused")
			d.requireNoKeyOpened(t)
			d.reopen(t)
			assert.Equal(t, c.reason, d.env.RunJobs(run)["deploy"].Reason)
		})
	}
}

func TestATargetThatDidNotChangeStartsNormallyAfterAHostKeyWasOnlyFetched(t *testing.T) {
	d := newDeployWorld(t)
	run := d.built(t, deployPipeline)
	require.Equal(t, "active", d.env.RunJobs(run)["deploy"].State)
	g := d.leaseDeploy(t)
	line, fingerprint := apptest.NewHostKeyLine(t)
	d.sql(t, `UPDATE "DeployTargetHosts" SET "PendingHostKey" = $1, "PendingFingerprint" = $2 WHERE "Host" = 'web-1.example.com'`, line, fingerprint)
	start, err := d.runner.Start(g)
	require.NoError(t, err, "a key that is only pending changes nothing")
	assert.Contains(t, openStart(t, d.runner, g, start), "dhow.target.0.key")
}

func TestARunStepNeverReceivesTheKeyOfTheTarget(t *testing.T) {
	d := newDeployWorld(t)
	run := d.built(t, deployPipeline)
	require.Equal(t, "active", d.env.RunJobs(run)["deploy"].State)
	spec := d.leaseDeploy(t).Grant.GetSpec()
	for _, ref := range spec.GetSecrets() {
		for i, step := range spec.GetSteps() {
			if step.GetPlugin() == nil {
				assert.NotContains(t, ref.GetStepIndexes(), int32(i), "no secret is for the run step %d", i)
			}
		}
		assert.NotEmpty(t, ref.GetStepIndexes(), "the key never has an empty step list, which would mean every step")
	}
}

func TestAnyHostIsRefusedForTheHealthCheckEvenWhenExtraEgressIsOpen(t *testing.T) {
	cases := map[string]string{
		"http://169.254.169.254/":     "HEALTH_URL_NOT_ALLOWED",
		"http://[fd00:ec2::254]/":     "HEALTH_URL_NOT_ALLOWED",
		"http://10.0.0.5/":            "HEALTH_URL_NOT_ALLOWED",
		"http://127.0.0.1:8080/":      "HEALTH_URL_NOT_ALLOWED",
		"http://[::1]/":               "HEALTH_URL_NOT_ALLOWED",
		"http://localhost/":           "HEALTH_URL_NOT_ALLOWED",
		"http://db.internal/":         "HEALTH_URL_NOT_ALLOWED",
		"https://status.example.org/": "",
		"https://" + hostTwo + "/":    "",
	}
	for url, want := range cases {
		t.Run(url, func(t *testing.T) {
			d := newDeployWorld(t)
			d.sql(t, `UPDATE "Environments" SET "AllowExtraDeployEgress" = true, "RuleVersion" = "RuleVersion" + 1 WHERE "Name" = 'production'`)
			run := d.built(t, strings.Replace(deployPipeline, "https://{host}/healthz", url, 1))
			deploy := d.env.RunJobs(run)["deploy"]
			if want == "" {
				assert.Equal(t, "active", deploy.State)
				return
			}
			assert.Equal(t, []string{"failed", want}, []string{deploy.State, deploy.Reason})
			d.requireNoKeyOpened(t)
		})
	}
}

const thirdPartyWithKey = `version: 1
on:
  push: {}
jobs:
  deploy:
    environment: production
    steps:
      - uses: ` + thirdParty + `
        with:
          token: ${{ secrets.WEB_SSH_KEY }}
`

func TestAThirdPartyPluginCannotNameTheKeyOfATarget(t *testing.T) {
	d := newDeployWorld(t)
	d.defs.Put(shaA, thirdPartyWithKey)
	run := d.push("refs/heads/main", shaA)
	d.env.StartRuns()
	deploy := d.env.RunJobs(run)["deploy"]
	assert.Equal(t, []string{"failed", "TARGET_KEY_NOT_REFERENCEABLE"}, []string{deploy.State, deploy.Reason})
	d.requireNoKeyOpened(t)
}

const otherKeyPipeline = `version: 1
on:
  push: {}
jobs:
  deploy:
    image: IMAGE
    environment: production
    steps:
      - run: ./deploy.sh
        env:
          KEY: ${{ secrets.OTHER_KEY }}
`

func TestAKeyThatBecomesTheKeyOfATargetAfterTheGateIsNotDelivered(t *testing.T) {
	d := newDeployWorld(t)
	d.env.SeedSecret(apptest.SecretRef(d.org, &d.project, "production"), "OTHER_KEY", "another-value")
	d.defs.Put(shaA, pipeline(otherKeyPipeline))
	run := d.push("refs/heads/main", shaA)
	d.env.StartRuns()
	require.Equal(t, "active", d.env.RunJobs(run)["deploy"].State)
	r, g := d.leaseKey(t, "deploy")
	d.sql(t, `UPDATE "DeployTargets" SET "KeySecretName" = 'OTHER_KEY', "Version" = "Version" + 1 WHERE "Name" = 'web'`)
	_, err := r.Start(g)
	require.Error(t, err)
	assert.Nil(t, d.lastUsed(d.org, &d.project, "OTHER_KEY"))
	d.reopen(t)
	assert.Equal(t, "TARGET_KEY_NOT_REFERENCEABLE", d.env.RunJobs(run)["deploy"].Reason)
}

func TestAKeyRotatedBeforeStartIsDeliveredAsTheNewVersion(t *testing.T) {
	d := newDeployWorld(t)
	run := d.built(t, deployPipeline)
	require.Equal(t, "active", d.env.RunJobs(run)["deploy"].State)
	g := d.leaseDeploy(t)
	d.env.SeedSecret(apptest.SecretRef(d.org, &d.project, "production"), keySecretName, "the-rotated-key-value")
	start, err := d.runner.Start(g)
	require.NoError(t, err)
	assert.Equal(t, "the-rotated-key-value", openStart(t, d.runner, g, start)["dhow.target.0.key"])
}

const forkTargetPipeline = `version: 1
on:
  pull_request: {}
jobs:
  deploy:
    steps:
      - uses: ` + sshPlugin + `
        with:
          artifact: web
          target: web
`

func TestAForkRunNeverGetsATarget(t *testing.T) {
	d := newDeployWorld(t)
	d.defs.Put(shaA, forkTargetPipeline)
	d.defs.Put(shaB, forkTargetPipeline)
	run := d.forkPR()
	maintainer, _ := d.env.SignedIn(d.org, "maintainer@example.com", "maintainer")
	d.env.StartRuns()
	require.Equal(t, 200, d.post(t, maintainer, run, "approve").Status)
	d.env.StartRuns()
	deploy := d.env.RunJobs(run)["deploy"]
	assert.Equal(t, []string{"failed", "FORK_PR_NO_ENVIRONMENT"}, []string{deploy.State, deploy.Reason})
	d.requireNoKeyOpened(t)
}

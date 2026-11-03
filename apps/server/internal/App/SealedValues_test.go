package app_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	app "github.com/Khaymah/dhow/apps/server/internal/App"
	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	backfill "github.com/Khaymah/dhow/apps/server/internal/Core/Backfill"
	appcrypto "github.com/Khaymah/dhow/apps/server/internal/Core/Crypto"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	instancesettings "github.com/Khaymah/dhow/apps/server/internal/Core/Settings"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
)

const (
	smtpPassword = "relay-credential-planted-for-m52"
	oidcSecret   = "Zq3vT8wLp2Rk9sXy4Nb7Hc1Jd6Fg0Ma5"
)

type sealedWorld struct {
	env      *apptest.Env
	triggers *apptest.FakeTriggers
	details  apptest.SetupDetails
	totp     apptest.TotpEnrollment
	org      uuid.UUID
	project  uuid.UUID
	ssh      apptest.SSHRepository
	hookPath string
	hookKey  string
}

func newSealedWorld(t *testing.T) sealedWorld {
	t.Helper()
	fake := &apptest.FakeTriggers{}
	env := apptest.NewWithOptions(t, apptest.Options{Triggers: fake})
	owner, d := env.SetupAndLogin()
	w := sealedWorld{env: env, triggers: fake, details: d, org: env.OrganizationID(d.OrgSlug)}
	w.totp = owner.EnableTotp(apptest.PasswordFactor(d.Password))
	env.Clock.Advance(31 * time.Second)
	user := uuid.MustParse(owner.UserID)
	linkedProject := env.SeedProject(w.org, user)
	res := owner.Do(apptest.Request{
		Method: http.MethodPut, Path: apptest.OrgPath(w.org, "/projects/"+linkedProject.String()+"/repository"),
		Body: map[string]any{"CloneUrl": "https://git.example.com/team/app.git"},
	})
	require.Equal(t, http.StatusCreated, res.Status, string(res.Raw))
	var linked struct {
		Repository    struct{ WebhookUrl string }
		WebhookSecret string
	}
	res.Decode(t, &linked)
	w.hookPath, w.hookKey = strings.TrimPrefix(linked.Repository.WebhookUrl, apptest.PublicOrigin), linked.WebhookSecret
	w.project = env.SeedProject(w.org, user)
	w.ssh = env.SeedSSHRepository(w.org, w.project, user)
	_, err := env.Pool.Exec(t.Context(), `UPDATE "Repositories" SET "PreviousDeployKeyPublic" = "DeployKeyPublic",
		"PreviousDeployKeySealed" = "DeployKeySealed", "PreviousDeployKeyExpiresAt" = now() + interval '1 day' WHERE "Id" = $1`, w.ssh.ID)
	require.NoError(t, err)
	w.seedSettings(t)
	return w
}

func (w sealedWorld) seedSettings(t *testing.T) {
	t.Helper()
	ctx := t.Context()
	secrets, err := w.env.Wired.Values.Column(instancesettings.SecretsSpec())
	require.NoError(t, err)
	mail, err := secrets.SealIn(ctx, sqlc.New(w.env.Pool), keys.InstanceOwner(), []byte("instance-settings:mail"), []byte(`{"Password":"`+smtpPassword+`"}`))
	require.NoError(t, err)
	oidcKeys, err := w.env.Wired.Values.Column(instancesettings.OidcSecretSpec())
	require.NoError(t, err)
	oidc, err := oidcKeys.SealIn(ctx, sqlc.New(w.env.Pool), keys.InstanceOwner(), []byte("instance-settings:oidc:corp"), []byte(oidcSecret))
	require.NoError(t, err)
	_, err = w.env.Pool.Exec(ctx, `INSERT INTO "InstanceSettings" ("Group", "Value", "Secrets")
		VALUES ('mail', '{"Host": "smtp.example.com", "From": "dhow@example.com", "Username": "dhow"}'::jsonb, $1)`, mail)
	require.NoError(t, err)
	_, err = w.env.Pool.Exec(ctx, `INSERT INTO "OidcProviders" ("ProviderId", "Type", "Name", "Issuer", "ClientId", "Secret")
		VALUES ('corp', 'oidc', 'Corp', 'https://id.example.com', 'dhow', $1)`, oidc)
	require.NoError(t, err)
	require.NoError(t, w.env.Wired.Settings.Load(ctx))
}

func makeLegacy(t *testing.T, env *apptest.Env) int {
	t.Helper()
	ctx := t.Context()
	n := 0
	all := env.AllValues(t)
	for _, l := range env.Registry().Locations() {
		col, err := env.Wired.Values.Column(l.Spec())
		require.NoError(t, err)
		legacy, err := appcrypto.NewSealer(env.Config.SecretKey, l.Spec().Label)
		require.NoError(t, err)
		for _, v := range all[l.Spec().Name()] {
			plain, err := col.Open(ctx, v.Owner, v.Context, v.Sealed)
			require.NoError(t, err, "%s row %s", l.Spec().Name(), v.Key)
			old, err := legacy.Seal(plain, v.Context)
			require.NoError(t, err)
			ok, err := l.Replace(ctx, env.Pool, v, old)
			require.NoError(t, err)
			require.True(t, ok)
			n++
		}
	}
	return n
}

func formats(t *testing.T, env *apptest.Env) map[byte]int {
	t.Helper()
	out := map[byte]int{}
	for _, values := range env.AllValues(t) {
		for _, v := range values {
			out[v.Sealed[0]]++
		}
	}
	return out
}

func signed(secret string, body []byte) map[string]string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(body)
	return map[string]string{"X-Gitea-Signature": hex.EncodeToString(h.Sum(nil)), "X-Gitea-Delivery": uuid.NewString(), "X-Gitea-Event": "push"}
}

func (w sealedWorld) everyValueWorks(t *testing.T, env *apptest.Env) {
	t.Helper()
	env.Clock.Advance(31 * time.Second)
	cookie := env.StartMfa(w.details.Email, w.details.Password)
	_, res := env.CompleteMfa(cookie, map[string]any{"Method": "totp", "Code": env.TotpCode(w.totp.Secret, 0)}, nil)
	require.Equal(t, http.StatusOK, res.Status, "sign-in with the same authenticator app: %s", res.Raw)

	cred, err := env.Wired.Sources.CheckoutCredential(t.Context(), w.org, w.project, w.ssh.ID)
	require.NoError(t, err)
	assert.Equal(t, w.ssh.PrivateKey, cred.Secret, "the checkout gets the same deploy key")

	after := strings.ReplaceAll(uuid.NewString(), "-", "") + "01234567"
	body := []byte(`{"ref":"refs/heads/main","before":"` + strings.Repeat("1", 40) + `","after":"` + after + `","pusher":{"login":"sara"}}`)
	hook := env.Webhook(http.MethodPost, w.hookPath, signed(w.hookKey, body), body)
	require.Equal(t, http.StatusAccepted, hook.Status, "the webhook signature still verifies: %s", hook.Raw)

	require.NoError(t, env.Wired.Settings.Load(t.Context()))
	assert.Equal(t, smtpPassword, env.Wired.Settings.Current().Values.Mail.Password)
	p, ok := env.Wired.Settings.Current().Oidc.Find("corp")
	require.True(t, ok)
	assert.Equal(t, oidcSecret, p.Config.ClientSecret)
}

func drainBackfills(t *testing.T, env *apptest.Env) *backfill.Runner {
	t.Helper()
	runner, err := app.NewBackfills(env.Pool, env.Wired.Values, nil)
	require.NoError(t, err)
	require.NoError(t, runner.Drain(t.Context()))
	require.NoError(t, backfill.RequireDone(t.Context(), env.Pool, runner.Names()...))
	return runner
}

func TestValuesSealedBeforeTheKeyringReadThenMoveToTheEnvelopeAndSurviveANewServerKey(t *testing.T) {
	w := newSealedWorld(t)
	assert.Equal(t, map[byte]int{keys.FormatVersion: 7}, formats(t, w.env), "new values use the envelope format")
	assert.Equal(t, 7, makeLegacy(t, w.env))
	require.Equal(t, map[byte]int{keys.LegacyFormat: 7}, formats(t, w.env))
	w.everyValueWorks(t, w.env)

	runner := drainBackfills(t, w.env)
	assert.Len(t, runner.Names(), len(w.env.Registry().Locations()), "one backfill per column")
	assert.Equal(t, map[byte]int{keys.FormatVersion: 7}, formats(t, w.env))
	w.everyValueWorks(t, w.env)

	moved := apptest.NewWithOptions(t, apptest.Options{Pool: w.env.Pool, Triggers: &apptest.FakeTriggers{}})
	require.NotEqual(t, w.env.Config.SecretKey, moved.Config.SecretKey)
	moved.Clock.Advance(w.env.Clock.Now().Sub(moved.Clock.Now()))
	w.everyValueWorks(t, moved)
}

func TestTheValueBackfillResumesAfterAnInterruption(t *testing.T) {
	w := newSealedWorld(t)
	makeLegacy(t, w.env)
	runner, err := app.NewBackfills(w.env.Pool, w.env.Wired.Values, nil)
	require.NoError(t, err)
	jobs, err := w.env.Registry().BackfillJobs(w.env.Wired.Values, nil)
	require.NoError(t, err)
	for _, job := range jobs[:3] {
		_, err := runner.Step(t.Context(), job)
		require.NoError(t, err)
	}
	half := formats(t, w.env)
	assert.Positive(t, half[keys.LegacyFormat])
	assert.Positive(t, half[keys.FormatVersion])
	w.everyValueWorks(t, w.env)

	drainBackfills(t, w.env)
	assert.Equal(t, map[byte]int{keys.FormatVersion: 7}, formats(t, w.env))
	list, err := backfill.List(t.Context(), w.env.Pool)
	require.NoError(t, err)
	var converted int64
	for _, p := range list {
		converted += p.Processed
	}
	assert.EqualValues(t, 7, converted, "every value converted once")
	w.everyValueWorks(t, w.env)
}

func TestAValueThatCannotBeOpenedStaysAndKeepsItsBackfillOpen(t *testing.T) {
	w := newSealedWorld(t)
	makeLegacy(t, w.env)
	_, err := w.env.Pool.Exec(t.Context(), `UPDATE "TotpFactors" SET "SecretSealed" = set_byte("SecretSealed", 20, get_byte("SecretSealed", 20) # 255)`)
	require.NoError(t, err)
	runner, err := app.NewBackfills(w.env.Pool, w.env.Wired.Values, nil)
	require.NoError(t, err)
	require.NoError(t, runner.Drain(t.Context()))
	name := keys.BackfillName(w.env.Registry().Locations()[0].Spec())
	assert.Equal(t, "keys.envelope.TotpFactors.SecretSealed", name)
	err = backfill.RequireDone(t.Context(), w.env.Pool, runner.Names()...)
	require.ErrorContains(t, err, name)
	assert.Equal(t, 1, formats(t, w.env)[keys.LegacyFormat])
}

var notSealed = map[string]string{
	"ApiTokens.TokenHash": "hash", "AuditEvents.Hash": "hash", "AuditEvents.PrevHash": "hash", "AuthChallenges.DeviceHash": "hash",
	"AuthChallenges.TokenHash": "hash", "GitSetupFlows.StateHash": "hash", "Invitations.TokenHash": "hash", "OidcFlows.DeviceHash": "hash",
	"OidcFlows.StateHash": "hash", "OidcFlows.TokenHash": "hash", "RecoveryCodes.CodeHash": "hash", "RunnerRegistrationTokens.TokenHash": "hash",
	"RunnerSessions.TokenHash": "hash", "Sessions.CsrfHash": "hash", "Sessions.TokenHash": "hash", "SetupTokens.TokenHash": "hash",
	"Artifacts.Digest": "digest", "BlobOwners.Digest": "digest", "BlobUploads.Digest": "digest", "Blobs.Digest": "digest",
	"CacheEntries.Digest": "digest", "JobAttempts.CompletionDigest": "digest", "Jobs.SpecDigest": "digest", "Leases.GrantDigest": "digest",
	"PipelineRuns.DefinitionDigest": "digest", "WebhookDeliveries.PayloadDigest": "digest",
	"Passkeys.Aaguid": "public", "Passkeys.CredentialId": "public", "Passkeys.PublicKey": "public key", "Users.WebauthnHandle": "public",
	"Runners.EncryptionPublicKey": "public key", "Runners.SigningPublicKey": "public key", "RunnerSessions.ChallengeNonce": "public nonce",
	"LogChunks.Data": "job logs, masked by the runner",
	"DataKeys.Wrapped": "the keyring wraps it", "KeyEncryptionKeys.CheckValue": "an HMAC of the KEK",
	"GitConnections.TokenSealed": "nothing writes it until M4.6, which must register it",
}

var maxCheck = regexp.MustCompile(`octet_length\("([A-Za-z]+)"\) <= (\d+)`)

func TestEverySealedColumnIsARegisteredLocationThatFitsItsCheck(t *testing.T) {
	env := apptest.New(t)
	reg := env.Registry()
	registered := map[string]keys.Spec{}
	for _, l := range reg.Locations() {
		registered[l.Spec().Name()] = l.Spec()
	}
	rows, err := env.Pool.Query(t.Context(), `SELECT c.table_name, c.column_name FROM information_schema.columns AS c
		WHERE c.table_schema = current_schema() AND c.data_type = 'bytea'
		  AND NOT EXISTS (SELECT 1 FROM pg_inherits WHERE inhrelid = to_regclass(quote_ident(c.table_name)))`)
	require.NoError(t, err)
	var found []string
	for rows.Next() {
		var table, column string
		require.NoError(t, rows.Scan(&table, &column))
		found = append(found, table+"."+column)
	}
	rows.Close()
	for _, name := range found {
		_, ok := registered[name]
		_, plain := notSealed[name]
		assert.True(t, ok || plain, "%s is a bytea column: register it as a value location, or add it to notSealed with the reason it holds no secret", name)
	}
	for name := range notSealed {
		assert.Contains(t, found, name, "notSealed lists a column that does not exist")
	}
	assert.Len(t, registered, 6)

	for name, spec := range registered {
		require.NoError(t, spec.Valid())
		var defs []string
		rows, err := env.Pool.Query(t.Context(), `SELECT pg_get_constraintdef(oid) FROM pg_constraint
			WHERE conrelid = to_regclass(quote_ident($1)) AND contype = 'c'`, spec.Table)
		require.NoError(t, err)
		for rows.Next() {
			var d string
			require.NoError(t, rows.Scan(&d))
			defs = append(defs, d)
		}
		rows.Close()
		limit := 0
		for _, d := range defs {
			for _, m := range maxCheck.FindAllStringSubmatch(d, -1) {
				if m[1] == spec.Column {
					limit, err = strconv.Atoi(m[2])
					require.NoError(t, err)
				}
			}
		}
		assert.Equal(t, spec.MaxSealed, limit, "%s: the spec names the CHECK of the column", name)
	}
}

func TestTheLargestRealValuesFitTheirColumns(t *testing.T) {
	env := apptest.New(t)
	owner, d := env.SetupAndLogin()
	org := env.OrganizationID(d.OrgSlug)
	user := uuid.MustParse(owner.UserID)
	repo := env.SeedSSHRepository(org, env.SeedProject(org, user), user)
	worst := map[string]int{
		"TotpFactors.SecretSealed":             auth.TotpSecretBytes,
		"Repositories.WebhookSecretSealed":     len(strings.Repeat("x", (config.WebhookSecretBytes*8+5)/6)),
		"Repositories.DeployKeySealed":         len(repo.PrivateKey) + len(config.DeployKeyComment),
		"Repositories.PreviousDeployKeySealed": len(repo.PrivateKey) + len(config.DeployKeyComment),
	}
	for _, l := range env.Registry().Locations() {
		spec := l.Spec()
		if n, ok := worst[spec.Name()]; ok {
			assert.LessOrEqual(t, n, spec.MaxPlaintext, spec.Name())
		}
		col, err := env.Wired.Values.Column(spec)
		require.NoError(t, err)
		owner := keys.Owner{Kind: spec.Owner}
		if spec.Owner != keys.OwnerInstance {
			owner.ID = uuid.New()
		}
		sealed, err := col.SealIn(context.Background(), sqlc.New(env.Pool), owner, []byte("row"), make([]byte, spec.MaxPlaintext))
		require.NoError(t, err)
		assert.LessOrEqual(t, len(sealed), spec.MaxSealed, spec.Name())
	}
}

func TestAnOldValuePlantedAfterTheBackfillIsRefused(t *testing.T) {
	w := newSealedWorld(t)
	drainBackfills(t, w.env)
	legacy, err := appcrypto.NewSealer(w.env.Config.SecretKey, config.TotpSealLabel)
	require.NoError(t, err)
	planted := make([]byte, auth.TotpSecretBytes)
	planted[0] = 7
	user := uuid.MustParse(w.userID(t))
	old, err := legacy.Seal(planted, []byte("totp:"+user.String()))
	require.NoError(t, err)
	_, err = w.env.Pool.Exec(t.Context(), `UPDATE "TotpFactors" SET "SecretSealed" = $1 WHERE "UserId" = $2`, old, user)
	require.NoError(t, err)

	for _, env := range []*apptest.Env{w.env, apptest.NewWithOptions(t, apptest.Options{Pool: w.env.Pool})} {
		env.Clock.Advance(w.env.Clock.Now().Sub(env.Clock.Now()) + 31*time.Second)
		cookie := env.StartMfa(w.details.Email, w.details.Password)
		_, res := env.CompleteMfa(cookie, map[string]any{"Method": "totp", "Code": env.TotpCode(planted, 0)}, nil)
		assert.NotEqual(t, http.StatusOK, res.Status, "a TOTP secret planted in the old format does not sign in")
	}
}

func (w sealedWorld) userID(t *testing.T) string {
	t.Helper()
	var id string
	require.NoError(t, w.env.Pool.QueryRow(t.Context(), `SELECT "UserId"::text FROM "TotpFactors" WHERE "ConfirmedAt" IS NOT NULL AND NOT "IsDeleted"`).Scan(&id))
	return id
}

package leaksweep_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"github.com/Khaymah/dhow/internal/leaksweep"
)

const (
	ingestPrefix = "dhow_ing_"
	tokenBody    = 43
)

func authorizedKey(t *testing.T) string {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	key, err := ssh.NewPublicKey(pub)
	require.NoError(t, err)
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))
}

func ingestToken(t *testing.T) string {
	t.Helper()
	raw := make([]byte, 32)
	_, err := rand.Read(raw)
	require.NoError(t, err)
	return ingestPrefix + base64.RawURLEncoding.EncodeToString(raw)[:tokenBody]
}

func targetKeys(t *testing.T) map[string][]byte {
	t.Helper()
	_, edKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	edBlock, err := ssh.MarshalPrivateKey(edKey, "target")
	require.NoError(t, err)
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	rsaBlock, err := ssh.MarshalPrivateKey(rsaKey, "target")
	require.NoError(t, err)
	return map[string][]byte{"ed25519": pem.EncodeToMemory(edBlock), "rsa": pem.EncodeToMemory(rsaBlock)}
}

func TestAPlantedTargetKeyIsFoundInEveryShapeItCanTake(t *testing.T) {
	for kind, text := range targetKeys(t) {
		t.Run(kind, func(t *testing.T) {
			require.True(t, leaksweep.LooksLikePrivateKey(text))
			needles, err := leaksweep.PrivateKey("target key", text)
			require.NoError(t, err)
			oneLine := strings.Join(strings.Split(strings.TrimSpace(string(text)), "\n")[1:], "")
			oneLine = strings.TrimSuffix(oneLine, "-----END OPENSSH PRIVATE KEY-----")
			dir := t.TempDir()
			planted := map[string]string{
				"clear.log":   string(text),
				"json.log":    fmt.Sprintf("{\"key\":%q}\n", text),
				"body.log":    "key=" + oneLine + "\n",
				"base64.log":  base64.StdEncoding.EncodeToString(text) + "\n",
				"audit.jsonl": `{"Data":{"Key":"` + strings.ReplaceAll(string(text), "\n", `\n`) + `"}}`,
			}
			for name, data := range planted {
				write(t, dir, name, []byte(data))
			}
			hits, _, err := leaksweep.Sweep([]string{dir}, needles, nil, leaksweep.DefaultLimits())
			require.NoError(t, err)
			found := map[string]bool{}
			for _, h := range hits {
				found[h.File[strings.LastIndex(h.File, "/")+1:]] = true
			}
			for name := range planted {
				assert.True(t, found[name], "the key planted in %s was not found", name)
			}
		})
	}
}

func TestThePublicHalfOfATargetKeyAndAFingerprintAreNotLeaks(t *testing.T) {
	text := targetKeys(t)["ed25519"]
	needles, err := leaksweep.PrivateKey("target key", text)
	require.NoError(t, err)
	pending, err := leaksweep.PendingHostKey("deploy.example.com", 22, authorizedKey(t))
	require.NoError(t, err)
	dir := t.TempDir()
	write(t, dir, "server.log", []byte(`{"msg":"host key pinned","Fingerprint":"SHA256:`+strings.Repeat("A", 43)+`","Public":"`+authorizedKey(t)+`"}`+"\n"))
	write(t, dir, "audit.jsonl", []byte(`{"Action":"deploy_target.host_key_confirmed","Data":{"HostId":"0192f0f0","Port":22}}`+"\n"))
	hits, _, err := leaksweep.Sweep([]string{dir}, append(needles, pending...), nil, leaksweep.DefaultLimits())
	require.NoError(t, err)
	assert.Empty(t, hits)
}

func TestAPendingHostKeyIsFoundEverywhereExceptItsOwnRow(t *testing.T) {
	key := authorizedKey(t)
	needles, err := leaksweep.PendingHostKey("deploy.example.com", 2222, key)
	require.NoError(t, err)
	blob := strings.Fields(key)[1]
	dir := t.TempDir()
	write(t, dir, "pg_dump/DeployTargetHosts.sql", []byte("1\tdeploy.example.com\t2222\t"+key+"\n"))
	write(t, dir, "pg_dump/AuditEvents.sql", []byte(`{"Data":{"Key":"`+blob+`"}}`+"\n"))
	write(t, dir, "server.log", []byte("[deploy.example.com]:2222 "+key+"\n"))
	write(t, dir, "api/targets.json", []byte(`{"Data":{"PendingHostKey":"`+key+`"}}`))
	write(t, dir, "events/bus.jsonl", []byte(base64.StdEncoding.EncodeToString([]byte(blob))+"\n"))
	hits, _, err := leaksweep.Sweep([]string{dir}, needles, nil, leaksweep.DefaultLimits())
	require.NoError(t, err)
	var files []string
	for _, h := range hits {
		files = append(files, strings.TrimPrefix(h.File, dir+"/"))
	}
	assert.ElementsMatch(t, []string{
		"pg_dump/AuditEvents.sql", "server.log", "server.log", "api/targets.json", "events/bus.jsonl",
	}, files)
}

func TestAPendingHostKeyThatIsNotAKeyIsRefused(t *testing.T) {
	for _, bad := range []string{"", "ssh-ed25519", "   "} {
		_, err := leaksweep.PendingHostKey("deploy.example.com", 22, bad)
		require.ErrorIs(t, err, leaksweep.ErrShortValue, bad)
	}
}

func TestAPlantedIngestTokenIsFoundByItsShapeAndOrdinaryTextIsNot(t *testing.T) {
	shape := leaksweep.Pattern{Name: "ingest token", Re: regexp.MustCompile(ingestPrefix + `[A-Za-z0-9_-]{20,}`)}
	token := ingestToken(t)
	listed, err := leaksweep.Encoded("ingest token", []byte(token))
	require.NoError(t, err)
	dir := t.TempDir()
	write(t, dir, "clear/server.log", []byte(`{"msg":"upload","Authorization":"Bearer `+token+`"}`+"\n"))
	write(t, dir, "encoded/server.log", []byte(base64.StdEncoding.EncodeToString([]byte("Bearer "+token))+"\n"))
	write(t, dir, "audit/rows.jsonl", []byte(`{"Data":{"TokenName":"ci","Prefix":"dhow_ing_"}}`+"\n"))
	write(t, dir, "api/tokens.json", []byte(`{"Data":[{"Name":"ci","Prefix":"dhow_ing","ExpiresAt":"2027-01-01T00:00:00Z"}]}`))
	hits, _, err := leaksweep.Sweep([]string{dir}, []leaksweep.Needle{listed}, []leaksweep.Pattern{shape}, leaksweep.DefaultLimits())
	require.NoError(t, err)
	var files []string
	for _, h := range hits {
		files = append(files, strings.TrimPrefix(h.File, dir+"/"))
	}
	assert.ElementsMatch(t, []string{"clear/server.log", "encoded/server.log"}, files)
}

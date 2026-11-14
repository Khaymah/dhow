//go:build leaksweep

package leaksweep_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/internal/leaksweep"
)

const (
	envNotify       = "DHOW_LEAK_SWEEP_EXPECT_NOTIFY"
	channelRowLabel = "NotificationChannels.ConfigSealed of row "
	decoySlack      = "https://hooks.slack.com/services/" + "T9DECOY001/B9DECOY001/" + "Zz9Yy8Xx7Ww6Vv5Uu4Tt3Ss2"
)

var plantPlaces = []string{
	"logs/server.log",
	"responses/notification-channels.json",
	"events/stream.txt",
	"pg_dump/NotificationDeliveries.sql",
	"inbox/notifications.json",
}

var cleanFiles = map[string]string{
	"logs/clean.log":                         `{"time":"2026-11-14T10:00:00Z","level":"INFO","msg":"notification sent","kind":"slack","host":"hooks.slack.com","state":"sent"}`,
	"responses/notification-rules.json":      `{"Success":true,"Data":{"Rules":[{"Events":["run.finished"],"Statuses":["failed"]}]}}`,
	"events/clean.txt":                       "id: 1-7\nevent: change\ndata: {\"Seq\":7,\"Type\":\"release.succeeded\"}\n\n",
	"pg_dump/NotificationChannels.sql":       "0198f5d2\tslack\tdemo\thooks.slack.com\t\\\\x0123abcdef\n",
	"inbox/clean.json":                       `{"Text":"Deployment failed","Url":"/projects/1/runs/2"}`,
	"responses/notification-deliveries.json": `{"Items":[{"State":"sent","LastError":"","LastStatus":200}]}`,
}

func channelSecrets(needles []leaksweep.Needle) map[string]leaksweep.Needle {
	out := map[string]leaksweep.Needle{}
	for _, n := range needles {
		rest, ok := strings.CutPrefix(n.Name, channelRowLabel)
		if !ok {
			continue
		}
		_, kind, found := strings.Cut(rest, " ")
		if !found || kind == "" {
			continue
		}
		if _, seen := out[kind]; !seen {
			out[kind] = n
		}
	}
	return out
}

func writeAt(t *testing.T, dir, name string, text []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, text, 0o600))
	return path
}

func TestEachNotificationSecretPlantedInAPlaceIsFoundThereAndNothingElseIs(t *testing.T) {
	w := buildWorld(t)
	secrets := channelSecrets(w.needles)
	if len(secrets) == 0 {
		require.Empty(t, os.Getenv(envNotify), "%s is set, but the demo made no notification channel with a secret", envNotify)
		t.Skip("the demo made no notification channel")
	}
	kinds := make([]string, 0, len(secrets))
	for k := range secrets {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	if os.Getenv(envNotify) != "" {
		for _, want := range []string{"secret", "bot token", "address"} {
			require.Contains(t, kinds, want, "the demo has no channel with a %s", want)
		}
	}
	dir := t.TempDir()
	want := map[string]string{}
	for i, kind := range kinds {
		place := plantPlaces[i%len(plantPlaces)]
		body := append(append([]byte(`{"msg":"x","value":"`), secrets[kind].Forms[0]...), []byte("\"}\n")...)
		want[writeAt(t, dir, strings.ReplaceAll(kind, " ", "-")+"/"+place, body)] = kind
	}
	decoy := writeAt(t, dir, "logs/other-channel.log", []byte(`{"msg":"x","value":"`+decoySlack+`"}`+"\n"))
	for name, text := range cleanFiles {
		writeAt(t, dir, name, []byte(text))
	}
	hits, _, err := leaksweep.Sweep([]string{dir}, w.needles, patterns(), leaksweep.DefaultLimits())
	require.NoError(t, err)
	found := map[string][]string{}
	for _, h := range hits {
		found[h.File] = append(found[h.File], h.Name)
	}
	for path, kind := range want {
		names := found[path]
		require.NotEmpty(t, names, "the %s planted in %s is not found", kind, path)
		var byValue bool
		for _, name := range names {
			byValue = byValue || strings.HasPrefix(name, channelRowLabel)
		}
		assert.True(t, byValue, "the %s planted in %s is found only by shape: %v", kind, path, names)
	}
	assert.Equal(t, []string{leaksweep.NameSlackWebhook}, found[decoy], "a Slack address of another channel is found by its shape and by nothing else")
	for path := range found {
		_, planted := want[path]
		assert.True(t, planted || path == decoy, "found something in %s, where nothing was planted: %v", path, found[path])
	}
	assert.Len(t, found, len(want)+1)
}

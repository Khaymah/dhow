package plugin

import (
	"path"
	"strings"
	"testing"
)

func FuzzManifest(f *testing.F) {
	f.Add(good, "channel", "#ops")
	f.Add(`{"Version":1,"Entrypoint":["/x"]}`, "a", "")
	f.Add(`{"Version":1,"Entrypoint":["x"]}`, "", "")
	f.Fuzz(func(t *testing.T, label, name, value string) {
		m, err := Parse(label)
		if err != nil {
			return
		}
		if len(m.Entrypoint) == 0 || !path.IsAbs(m.Entrypoint[0]) {
			t.Fatalf("an accepted manifest has entrypoint %q", m.Entrypoint)
		}
		env, err := m.Env(map[string]string{name: value})
		if err != nil {
			return
		}
		for k := range env {
			if !strings.HasPrefix(k, InputEnvPrefix) {
				t.Fatalf("an input became %q", k)
			}
		}
	})
}

func FuzzOutputs(f *testing.F) {
	f.Add([]byte("url=https://example.com\n"))
	f.Add([]byte("a=b\nc=d=e\n"))
	f.Add([]byte("bad\n"))
	f.Fuzz(func(t *testing.T, raw []byte) {
		got, err := ParseOutputs(raw, nil)
		if err != nil {
			return
		}
		if len(got) > MaxOutputs {
			t.Fatalf("%d outputs", len(got))
		}
		for k, v := range got {
			if !outputPattern.MatchString(k) || len(v) > MaxOutputValue || strings.ContainsAny(v, "\r\n\x00") {
				t.Fatalf("accepted %q=%q", k, v)
			}
		}
	})
}

package resolved_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Khaymah/dhow/apps/plugins/deploy-compose/internal/policy"
	"github.com/Khaymah/dhow/apps/plugins/deploy-compose/internal/resolved"
)

const (
	image   = "reg.example.com/shop/web@sha256:0000000000000000000000000000000000000000000000000000000000000001"
	release = "/srv/shop/releases/r1"
)

func expect() resolved.Expect {
	return resolved.Expect{Project: "shop", ReleaseDir: release, Images: map[string]string{"web": image}}
}

func model(edit func(m, web map[string]any)) []byte {
	web := map[string]any{"image": image, "volumes": []any{
		map[string]any{"type": "bind", "source": release + "/conf", "target": "/conf", "read_only": true},
		map[string]any{"type": "volume", "source": "data", "target": "/data"},
	}}
	m := map[string]any{
		"name": "shop", "services": map[string]any{"web": web},
		"volumes":  map[string]any{"data": map[string]any{"name": "shop_data"}},
		"networks": map[string]any{"default": map[string]any{"name": "shop_default", "ipam": map[string]any{}}},
	}
	if edit != nil {
		edit(m, web)
	}
	raw, _ := json.MarshalIndent(m, "", "  ")
	return append([]byte("WARN[0000] a warning\n"), raw...)
}

func code(t *testing.T, err error) string {
	t.Helper()
	var v *policy.Violation
	require.True(t, errors.As(err, &v), "%v", err)
	return v.Code
}

func TestAModelThatIsWhatWasCheckedPasses(t *testing.T) {
	require.NoError(t, resolved.Check(model(nil), expect()))
}

func TestWhateverTheParserMissedFailsClosedAfterInterpolation(t *testing.T) {
	for name, edit := range map[string]func(m, web map[string]any){
		"another project": func(m, _ map[string]any) { m["name"] = "other" },
		"no project":      func(m, _ map[string]any) { delete(m, "name") },
		"another image": func(_, web map[string]any) {
			web["image"] = "reg.example.com/shop/web@sha256:" + "f" + image[len(image)-63:]
		},
		"an image by tag":      func(_, web map[string]any) { web["image"] = "reg.example.com/shop/web:latest" },
		"a build":              func(_, web map[string]any) { web["build"] = map[string]any{"context": "."} },
		"an extra service":     func(m, web map[string]any) { m["services"].(map[string]any)["evil"] = web },
		"a missing service":    func(m, _ map[string]any) { m["services"] = map[string]any{} },
		"privileged":           func(_, web map[string]any) { web["privileged"] = true },
		"cap_add":              func(_, web map[string]any) { web["cap_add"] = []any{"SYS_ADMIN"} },
		"devices":              func(_, web map[string]any) { web["devices"] = []any{"/dev/sda"} },
		"security_opt":         func(_, web map[string]any) { web["security_opt"] = []any{"seccomp=unconfined"} },
		"pid host":             func(_, web map[string]any) { web["pid"] = "host" },
		"ipc host":             func(_, web map[string]any) { web["ipc"] = "host" },
		"uts host":             func(_, web map[string]any) { web["uts"] = "host" },
		"ipc of a container":   func(_, web map[string]any) { web["ipc"] = "container:x" },
		"network of a service": func(_, web map[string]any) { web["network_mode"] = "service:db" },
		"network bridge":       func(_, web map[string]any) { web["network_mode"] = "bridge" },
		"network host":         func(_, web map[string]any) { web["network_mode"] = "host" },
		"the api socket":       func(_, web map[string]any) { web["use_api_socket"] = true },
		"volumes_from":         func(_, web map[string]any) { web["volumes_from"] = []any{"db"} },
		"a relative bind": func(_, web map[string]any) {
			web["volumes"] = []any{map[string]any{"type": "bind", "source": "conf", "read_only": true}}
		},
		"a bind outside": func(_, web map[string]any) {
			web["volumes"] = []any{map[string]any{"type": "bind", "source": "/etc", "read_only": true}}
		},
		"a sibling of the dir": func(_, web map[string]any) {
			web["volumes"] = []any{map[string]any{"type": "bind", "source": release + "-x/c", "read_only": true}}
		},
		"the release itself": func(_, web map[string]any) {
			web["volumes"] = []any{map[string]any{"type": "bind", "source": release, "read_only": true}}
		},
		"a dotdot out": func(_, web map[string]any) {
			web["volumes"] = []any{map[string]any{"type": "bind", "source": release + "/../x", "read_only": true}}
		},
		"a writable bind": func(_, web map[string]any) {
			web["volumes"] = []any{map[string]any{"type": "bind", "source": release + "/conf", "read_only": false}}
		},
		"a bind without the key": func(_, web map[string]any) {
			web["volumes"] = []any{map[string]any{"type": "bind", "source": release + "/conf"}}
		},
		"a named pipe":    func(_, web map[string]any) { web["volumes"] = []any{map[string]any{"type": "npipe", "source": "x"}} },
		"a volume driver": func(m, _ map[string]any) { m["volumes"] = map[string]any{"data": map[string]any{"driver": "nfs"}} },
		"volume options": func(m, _ map[string]any) {
			m["volumes"] = map[string]any{"data": map[string]any{"driver_opts": map[string]any{"device": "/"}}}
		},
		"an external volume":  func(m, _ map[string]any) { m["volumes"] = map[string]any{"data": map[string]any{"external": true}} },
		"an external network": func(m, _ map[string]any) { m["networks"] = map[string]any{"n": map[string]any{"external": true}} },
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, resolved.Code, code(t, resolved.Check(model(edit), expect())))
		})
	}
}

func TestTheAdministratorOptionRelaxesTheSecurityFieldsButNotTheImagesTheProjectOrTheVolumes(t *testing.T) {
	e := expect()
	e.AllowUnsafe = true
	for name, edit := range map[string]func(m, web map[string]any){
		"privileged":   func(_, web map[string]any) { web["privileged"] = true },
		"network host": func(_, web map[string]any) { web["network_mode"] = "host" },
		"a writable bind": func(_, web map[string]any) {
			web["volumes"] = []any{map[string]any{"type": "bind", "source": "/srv/shop/shared/uploads", "read_only": false}}
		},
	} {
		assert.NoError(t, resolved.Check(model(edit), e), name)
	}
	for name, edit := range map[string]func(m, web map[string]any){
		"another project": func(m, _ map[string]any) { m["name"] = "other" },
		"another image":   func(_, web map[string]any) { web["image"] = "x@sha256:" + image[len(image)-64:] },
		"a volume driver": func(m, _ map[string]any) { m["volumes"] = map[string]any{"data": map[string]any{"driver": "nfs"}} },
		"a relative bind": func(_, web map[string]any) { web["volumes"] = []any{map[string]any{"type": "bind", "source": "conf"}} },
	} {
		assert.Equal(t, resolved.Code, code(t, resolved.Check(model(edit), e)), name)
	}
}

func TestOutputThatIsNotAModelIsRefused(t *testing.T) {
	for _, out := range []string{"", "WARN only\n", "{", "[]", "null", "WARN\n{\"name\": "} {
		assert.Equal(t, resolved.Code, code(t, resolved.Check([]byte(out), expect())), out)
	}
}

func composeAvailable() bool {
	return exec.CommandContext(context.Background(), "docker", "compose", "version").Run() == nil
}

func realModel(t *testing.T, dir string) []byte {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "env", "-i", "PATH="+os.Getenv("PATH"), "HOME="+os.Getenv("HOME"), "docker", "compose",
		"--project-name=shop", "--project-directory="+dir, "--file="+filepath.Join(dir, "compose.yaml"), "--file="+filepath.Join(dir, "compose.dhow.yaml"),
		"--env-file=/dev/null", "config", "--format", "json")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return out
}

func TestRealComposeOutputOfAcceptedFilesPassesAndOfDangerousFilesDoesNot(t *testing.T) {
	if !composeAvailable() {
		t.Skip("docker compose is not available, so the differential test against it is skipped")
	}
	accepted := []string{
		"services:\n  web:\n    image: " + image + "\n    volumes:\n      - ./conf:/conf:ro\n      - data:/data\n    networks: [back]\n    environment:\n      - A=${A:-x}\n    ports: ['8080:80']\n    labels:\n      traefik.enable: 'true'\nnetworks:\n  back: {}\nvolumes:\n  data: {}\n",
		"services:\n  web:\n    image: " + image + "\n    read_only: true\n    tmpfs: [/tmp]\n    command: ['run', '$$HOME']\n    healthcheck:\n      test: ['CMD', 'true']\n    init: true\n    volumes:\n      - /cache\n",
		"name: shop\nservices:\n  web:\n    image: " + image + "\n    volumes:\n      - type: bind\n        source: ./conf\n        target: /conf\n        read_only: true\n",
	}
	for i, file := range accepted {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "conf"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(file), 0o644))
		res, err := policy.Check([]byte(file), policy.Options{DeployPath: "/srv/shop", Project: "shop"})
		require.NoError(t, err, "case %d", i)
		override, err := policy.Override(res.Images)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "compose.dhow.yaml"), override, 0o644))
		require.NoError(t, resolved.Check(realModel(t, dir), resolved.Expect{Project: "shop", ReleaseDir: dir, Images: res.Images}), "case %d", i)
	}

	dangerous := map[string]string{
		"privileged":      "    privileged: true\n",
		"network host":    "    network_mode: host\n",
		"cap_add":         "    cap_add: [SYS_ADMIN]\n",
		"pid host":        "    pid: host\n",
		"a writable bind": "    volumes:\n      - ./conf:/conf\n",
	}
	for name, extra := range dangerous {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "conf"), 0o755))
		file := "services:\n  web:\n    image: " + image + "\n" + extra
		require.NoError(t, os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(file), 0o644))
		override, err := policy.Override(map[string]string{"web": image})
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "compose.dhow.yaml"), override, 0o644))
		assert.Equal(t, resolved.Code, code(t, resolved.Check(realModel(t, dir), resolved.Expect{Project: "shop", ReleaseDir: dir, Images: map[string]string{"web": image}})), name)
	}
}

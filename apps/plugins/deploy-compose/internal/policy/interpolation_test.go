package policy_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Khaymah/dhow/apps/plugins/deploy-compose/internal/policy"
)

func TestAVariableOutsideTheValueFieldsIsRefusedEvenWhenUnsafeIsOpen(t *testing.T) {
	for name, extra := range map[string]string{
		"read_only with a default":               "    volumes:\n      - type: bind\n        source: ./data\n        target: /data\n        read_only: \"${RO:-false}\"\n",
		"read_only of the service":               "    read_only: \"${RO:-false}\"\n",
		"privileged with a default":              "    privileged: \"${P:-false}\"\n",
		"a variable in ports":                    "    ports: ['${PORT}:80']\n",
		"a variable in networks":                 "    networks: ['${NET}']\n",
		"a variable in restart":                  "    restart: ${R}\n",
		"a variable in user":                     "    user: '${U}'\n",
		"a variable in depends_on":               "    depends_on: ['${D}']\n",
		"a variable in a mode string":            "    volumes: ['./c:/c:ro,${X}']\n",
		"a variable in a source":                 "    volumes: ['${S}:/c:ro']\n",
		"a variable in env_file":                 "    env_file: ${F}\n",
		"a variable in a key":                    "    ulimits:\n      ${K}: 1\n",
		"a variable in deploy":                   "    deploy:\n      resources:\n        limits:\n          memory: ${M}\n",
		"a variable in a top key":                "x-top: ${X}\n",
		"a variable in the top volumes":          "volumes:\n  v:\n    labels:\n      a: ${X}\n",
		"a variable in logging":                  "    logging:\n      driver: json-file\n      options:\n        max-size: ${S}\n",
		"a variable in expose":                   "    expose: ['${E}']\n",
		"a variable in stop signal":              "    stop_signal: ${SIG}\n",
		"a variable in the image of an override": "    hostname: ${H}\n",
	} {
		t.Run(name, func(t *testing.T) {
			for _, allow := range []bool{false, true} {
				_, code := check(t, compose(extra), policy.Options{AllowUnsafe: allow})
				assert.Equal(t, "COMPOSE_UNSAFE_INTERPOLATION", code, "allow=%v", allow)
			}
		})
	}
}

func TestAVariableIsAllowedOnlyInTheContainerValues(t *testing.T) {
	for name, extra := range map[string]string{
		"environment as a list":    "    environment:\n      - A=${A}\n      - B=${B:-x}\n",
		"environment as a map":     "    environment:\n      A: ${A}\n      ${K}: v\n",
		"command":                  "    command: run ${A}\n",
		"command as a list":        "    command: ['run', '${A}']\n",
		"entrypoint":               "    entrypoint: ['${A}']\n",
		"labels":                   "    labels:\n      a: ${A}\n",
		"labels as a list":         "    labels:\n      - a=${A}\n",
		"healthcheck test":         "    healthcheck:\n      test: ['CMD', 'x', '${A}']\n",
		"healthcheck test as text": "    healthcheck:\n      test: curl ${A}\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, code := check(t, compose(extra), policy.Options{})
			assert.Empty(t, code)
		})
	}
	_, code := check(t, compose("    healthcheck:\n      interval: ${I}\n"), policy.Options{})
	assert.Equal(t, "COMPOSE_UNSAFE_INTERPOLATION", code, "only healthcheck.test is a value field")
}

func TestADoubleDollarIsLiteralAndATripleIsNot(t *testing.T) {
	for _, ok := range []string{"    user: 'a$$b'\n", "    restart: '$$'\n", "    ports: ['8$$0:80']\n", "    hostname: '$$$$x'\n"} {
		_, code := check(t, compose(ok), policy.Options{})
		assert.Empty(t, code, ok)
	}
	for _, bad := range []string{"    user: '$$$X'\n", "    user: '$$ $X'\n", "    user: 'x$'\n", "    user: '$'\n", "    user: '$$$'\n"} {
		_, code := check(t, compose(bad), policy.Options{})
		assert.Equal(t, "COMPOSE_UNSAFE_INTERPOLATION", code, bad)
	}
}

func TestATypedFieldMustBeALiteralYamlType(t *testing.T) {
	for name, extra := range map[string]string{
		"read_only as text":          "    read_only: 'true'\n",
		"privileged as text":         "    privileged: 'false'\n",
		"privileged as a number":     "    privileged: 1\n",
		"init as text":               "    init: 'yes'\n",
		"use_api_socket as text":     "    use_api_socket: 'false'\n",
		"a mount read_only as text":  "    volumes:\n      - type: bind\n        source: ./c\n        target: /c\n        read_only: 'true'\n",
		"a mount source as a number": "    volumes:\n      - type: bind\n        source: 5\n        target: /c\n",
		"a hook privileged as text":  "    post_start:\n      - command: id\n        privileged: 'false'\n",
		"external as text":           "volumes:\n  v:\n    external: 'false'\n",
	} {
		t.Run(name, func(t *testing.T) {
			for _, allow := range []bool{false, true} {
				_, code := check(t, compose(extra), policy.Options{AllowUnsafe: allow})
				assert.Equal(t, policy.CodeInvalid, code, "allow=%v", allow)
			}
		})
	}
	_, code := check(t, compose("    read_only: true\n    init: false\n"), policy.Options{})
	assert.Empty(t, code)
}

func TestTheModeOfAShortVolumeIsOneOfTheKnownFlags(t *testing.T) {
	for entry, want := range map[string]string{
		"./c:/c:ro":            "",
		"./c:/c:ro,private":    "",
		"./c:/c:ro,z":          policy.CodeInvalid,
		"./c:/c:ro,Z,rprivate": policy.CodeInvalid,
		"./c:/c:shared":        policy.CodeInvalid,
		"./c:/c:rslave":        policy.CodeInvalid,
		"data:/d:rw":           "",
		"data:/d:nocopy":       "",
		"./c:/c:bogus":         policy.CodeInvalid,
		"./c:/c:ro,bogus":      policy.CodeInvalid,
		"./c:/c:":              policy.CodeInvalid,
		"./c:/c:ro:extra":      policy.CodeInvalid,
		"./c:/c:ro:extra:more": policy.CodeInvalid,
		"./c:/c:RO":            policy.CodeInvalid,
		"data:/d:bogus":        policy.CodeInvalid,
		"./c:/c:ro,rw":         "COMPOSE_UNSAFE_BIND_WRITABLE",
		"./c:/c:rw":            "COMPOSE_UNSAFE_BIND_WRITABLE",
		"./c:/c:z":             policy.CodeInvalid,
	} {
		_, code := check(t, compose(vol(entry)+"volumes:\n  data: {}\n"), policy.Options{})
		assert.Equal(t, want, code, entry)
	}
}

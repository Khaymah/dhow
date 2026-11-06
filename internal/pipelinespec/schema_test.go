package pipelinespec

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type jsonSchema map[string]any

func loadSchema(t *testing.T) jsonSchema {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "packages", "spec", "dhow.schema.json"))
	require.NoError(t, err)
	var s jsonSchema
	require.NoError(t, json.Unmarshal(raw, &s))
	return s
}

func (root jsonSchema) resolve(t *testing.T, node jsonSchema) jsonSchema {
	t.Helper()
	ref, ok := node["$ref"].(string)
	if !ok {
		return node
	}
	name, found := strings.CutPrefix(ref, "#/$defs/")
	require.True(t, found, "unsupported $ref %s", ref)
	defs, ok := root["$defs"].(map[string]any)
	require.True(t, ok)
	def, ok := defs[name].(map[string]any)
	require.True(t, ok, "missing definition %s", name)
	return root.resolve(t, def)
}

func child(t *testing.T, node jsonSchema, key, path string) jsonSchema {
	t.Helper()
	v, ok := node[key].(map[string]any)
	require.True(t, ok, "%s: schema has no %s object", path, key)
	return v
}

func schemaTypes(node jsonSchema) []string {
	switch v := node["type"].(type) {
	case string:
		return []string{v}
	case []any:
		out := make([]string, 0, len(v))
		for _, x := range v {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func compareSchema(t *testing.T, root, node jsonSchema, typ reflect.Type, path string) {
	t.Helper()
	node = root.resolve(t, node)
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	switch typ {
	case scalarType:
		assert.Contains(t, schemaTypes(node), "string", path)
		return
	case matrixType:
		props := child(t, node, "properties", path)
		assert.ElementsMatch(t, []string{matrixInclude, matrixExclude}, keysOf(props), path)
		axis := child(t, node, "additionalProperties", path)
		assert.Contains(t, schemaTypes(axis), "array", path)
		return
	}
	switch typ.Kind() {
	case reflect.Struct:
		props := child(t, node, "properties", path)
		fields := yamlFields(typ)
		assert.ElementsMatch(t, keysOf(fields), keysOf(props), "%s: Go fields and schema properties differ", path)
		assert.Equal(t, false, node["additionalProperties"], "%s: unknown keys must be refused", path)
		for name, ft := range fields {
			if p, ok := props[name].(map[string]any); ok {
				compareSchema(t, root, p, ft, joinPath(path, name))
			}
		}
	case reflect.Map:
		compareSchema(t, root, child(t, node, "additionalProperties", path), typ.Elem(), path+".*")
	case reflect.Slice:
		assert.Contains(t, schemaTypes(node), "array", path)
		compareSchema(t, root, child(t, node, "items", path), typ.Elem(), path+"[]")
	case reflect.String:
		_, enum := node["enum"]
		assert.True(t, enum || slices.Contains(schemaTypes(node), "string"), "%s: expected a string", path)
	case reflect.Int:
		_, isConst := node["const"]
		assert.True(t, isConst || slices.Contains(schemaTypes(node), "integer"), "%s: expected an integer", path)
	case reflect.Bool:
		assert.Contains(t, schemaTypes(node), "boolean", path)
	default:
		t.Errorf("%s: unexpected Go kind %s", path, typ.Kind())
	}
}

func keysOf[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func TestSchemaMatchesTheValidator(t *testing.T) {
	root := loadSchema(t)
	compareSchema(t, root, root, specType, "")
	assert.ElementsMatch(t, []any{"version", "on", "jobs"}, root["required"])
}

func TestSchemaListsOnlySupportedTriggers(t *testing.T) {
	root := loadSchema(t)
	triggers := root.resolve(t, jsonSchema{"$ref": "#/$defs/triggers"})
	props := child(t, triggers, "properties", "on")
	assert.Contains(t, props, "artifact")
	assert.False(t, deferredTrigger("artifact"))
	for _, deferred := range []string{"schedule"} {
		assert.NotContains(t, props, deferred)
		assert.True(t, deferredTrigger(deferred))
	}
}

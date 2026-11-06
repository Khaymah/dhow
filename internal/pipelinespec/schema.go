package pipelinespec

import (
	"math"
	"reflect"
	"strings"

	"go.yaml.in/yaml/v3"
)

var (
	specType     = reflect.TypeFor[Spec]()
	triggersType = reflect.TypeFor[Triggers]()
	scalarType   = reflect.TypeFor[Scalar]()
	matrixType   = reflect.TypeFor[Matrix]()
	axisType     = reflect.TypeFor[[]Scalar]()
	entriesType  = reflect.TypeFor[[]map[string]Scalar]()
)

func deferredTrigger(key string) bool {
	return key == "schedule"
}

type schemaWalk struct {
	issues *issueList
}

func isNull(n *yaml.Node) bool {
	return n.Kind == yaml.ScalarNode && n.Tag == tagNull
}

func (w *schemaWalk) mark(path string, n *yaml.Node) {
	if _, ok := w.issues.positions[path]; !ok {
		w.issues.positions[path] = nodePos(n)
	}
}

func (w *schemaWalk) walk(n *yaml.Node, t reflect.Type, path string) {
	if w.issues.full() {
		return
	}
	w.mark(path, n)
	pointer := t.Kind() == reflect.Pointer
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if isNull(n) {
		if pointer && t.Kind() == reflect.Struct && t != scalarType && t != matrixType {
			n.Kind, n.Tag, n.Value = yaml.MappingNode, tagMap, ""
		}
		return
	}
	switch t {
	case scalarType:
		if _, err := scalarFromNode(n); err != nil {
			w.issues.add(CodeTypeMismatch, path)
		}
		return
	case matrixType:
		w.matrix(n, path)
		return
	}
	w.kind(n, t, path)
}

func (w *schemaWalk) kind(n *yaml.Node, t reflect.Type, path string) {
	switch t.Kind() {
	case reflect.String:
		if n.Kind != yaml.ScalarNode {
			w.issues.add(CodeTypeMismatch, path)
		}
	case reflect.Int:
		var v int64
		if n.Kind != yaml.ScalarNode || n.Tag != tagInt || n.Decode(&v) != nil || v > math.MaxInt32 || v < math.MinInt32 {
			w.issues.add(CodeTypeMismatch, path)
		}
	case reflect.Bool:
		if n.Kind != yaml.ScalarNode || n.Tag != tagBool {
			w.issues.add(CodeTypeMismatch, path)
		}
	case reflect.Slice:
		w.sequence(n, t.Elem(), path)
	case reflect.Map:
		w.mapping(n, t.Elem(), path)
	case reflect.Struct:
		w.object(n, t, path)
	default:
		w.issues.add(CodeTypeMismatch, path)
	}
}

func (w *schemaWalk) sequence(n *yaml.Node, elem reflect.Type, path string) {
	if n.Kind != yaml.SequenceNode {
		w.issues.add(CodeTypeMismatch, path)
		return
	}
	for i, c := range n.Content {
		child := indexPath(path, i)
		if isNull(c) {
			w.mark(child, c)
			w.issues.add(CodeTypeMismatch, child)
			continue
		}
		w.walk(c, elem, child)
	}
}

func (w *schemaWalk) mapping(n *yaml.Node, elem reflect.Type, path string) {
	if n.Kind != yaml.MappingNode {
		w.issues.add(CodeTypeMismatch, path)
		return
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		key, val := n.Content[i], n.Content[i+1]
		child := joinPath(path, key.Value)
		w.mark(child, key)
		w.walk(val, elem, child)
	}
}

func (w *schemaWalk) object(n *yaml.Node, t reflect.Type, path string) {
	if n.Kind != yaml.MappingNode {
		w.issues.add(CodeTypeMismatch, path)
		return
	}
	fields := yamlFields(t)
	for i := 0; i+1 < len(n.Content); i += 2 {
		key, val := n.Content[i], n.Content[i+1]
		child := joinPath(path, key.Value)
		w.mark(child, key)
		ft, ok := fields[key.Value]
		switch {
		case !ok && t == triggersType && deferredTrigger(key.Value):
			w.issues.add(CodeTriggerNotSupported, child)
		case !ok:
			w.issues.add(CodeUnknownKey, child)
		default:
			w.walk(val, ft, child)
		}
	}
}

func (w *schemaWalk) matrix(n *yaml.Node, path string) {
	if n.Kind != yaml.MappingNode {
		w.issues.add(CodeTypeMismatch, path)
		return
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		key, val := n.Content[i], n.Content[i+1]
		child := joinPath(path, key.Value)
		w.mark(child, key)
		if isNull(val) {
			w.issues.add(CodeTypeMismatch, child)
			continue
		}
		if key.Value == matrixInclude || key.Value == matrixExclude {
			w.walk(val, entriesType, child)
			continue
		}
		w.walk(val, axisType, child)
	}
}

func yamlFields(t reflect.Type) map[string]reflect.Type {
	out := map[string]reflect.Type{}
	for f := range t.Fields() {
		if !f.IsExported() {
			continue
		}
		name, _, _ := strings.Cut(f.Tag.Get("yaml"), ",")
		if name == "" || name == "-" {
			continue
		}
		out[name] = f.Type
	}
	return out
}

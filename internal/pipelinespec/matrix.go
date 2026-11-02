package pipelinespec

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/cel-go/cel"
)

type MatrixValue struct {
	Key   string
	Value Scalar
}

type combination []MatrixValue

func (c combination) get(key string) (Scalar, bool) {
	for _, v := range c {
		if v.Key == key {
			return v.Value, true
		}
	}
	return Scalar{}, false
}

func runJobKey(id string, c combination) string {
	if len(c) == 0 {
		return id
	}
	parts := make([]string, 0, len(c))
	for _, v := range c {
		parts = append(parts, v.Key+"="+keyEscaper.Replace(v.Value.Text))
	}
	return id + " (" + strings.Join(parts, ", ") + ")"
}

var keyEscaper = strings.NewReplacer(`\`, `\\`, ",", `\,`, "=", `\=`, "(", `\(`, ")", `\)`)

func sameScalar(a, b Scalar) bool {
	return a.Kind == b.Kind && a.Value() == b.Value()
}

func validMatrixValue(s Scalar) bool {
	if s.Kind == 0 || utf8.RuneCountInString(s.Text) > MaxMatrixValue || !utf8.ValidString(s.Text) {
		return false
	}
	return strings.IndexFunc(s.Text, unicode.IsControl) < 0 && !strings.Contains(s.Text, exprOpen)
}

func validMatrixKey(k string) bool {
	return len(k) <= 64 && celIdent(k) && k != matrixInclude && k != matrixExclude
}

type matrixExpansion struct {
	issues *issueList
	path   string
	m      *Matrix
	keys   []string
	ok     bool
}

func expandMatrix(m *Matrix, path string, issues *issueList) ([]combination, bool) {
	if m == nil {
		return []combination{{}}, true
	}
	e := &matrixExpansion{issues: issues, path: path, m: m, keys: sortedNames(m.Axes), ok: true}
	raw := e.axes()
	e.entries(m.Exclude, matrixExclude, false)
	e.entries(m.Include, matrixInclude, true)
	if !e.ok {
		return nil, false
	}
	if raw > MaxMatrixRaw || len(m.Include) > MaxMatrixJobs || len(m.Exclude) > MaxMatrixJobs {
		issues.add(CodeMatrixTooLarge, path)
		return nil, false
	}
	combos := e.product()
	combos = slices.DeleteFunc(combos, func(c combination) bool {
		return slices.ContainsFunc(m.Exclude, func(x map[string]Scalar) bool { return matches(c, x) })
	})
	for _, inc := range m.Include {
		combos = append(combos, fromEntry(inc))
	}
	return e.finish(combos)
}

func (e *matrixExpansion) axes() int {
	if len(e.keys) > MaxMatrixAxes {
		e.issues.add(CodeMatrixTooLarge, e.path)
		e.ok = false
		return 0
	}
	raw := 1
	for _, k := range e.keys {
		p := joinPath(e.path, k)
		if !validMatrixKey(k) {
			e.issues.add(CodeMatrixKeyInvalid, p)
			e.ok = false
		}
		values := e.m.Axes[k]
		if len(values) == 0 {
			e.issues.add(CodeMatrixEmpty, p)
			e.ok = false
		}
		for i, v := range values {
			if !validMatrixValue(v) {
				e.issues.add(CodeMatrixValueInvalid, indexPath(p, i))
				e.ok = false
			}
		}
		if raw <= MaxMatrixRaw {
			raw *= max(len(values), 1)
		}
	}
	return raw
}

func (e *matrixExpansion) entries(list []map[string]Scalar, name string, include bool) {
	p := joinPath(e.path, name)
	var shape []string
	for i, entry := range list {
		ep := indexPath(p, i)
		keys := sortedNames(entry)
		switch {
		case len(keys) == 0:
			e.issues.add(CodeMatrixEntryInvalid, ep)
			e.ok = false
		case include && len(e.keys) > 0 && !slices.Equal(keys, e.keys):
			e.issues.add(CodeMatrixEntryInvalid, ep)
			e.ok = false
		case include && len(e.keys) == 0 && shape != nil && !slices.Equal(keys, shape):
			e.issues.add(CodeMatrixEntryInvalid, ep)
			e.ok = false
		case !include && slices.ContainsFunc(keys, func(k string) bool { _, ok := e.m.Axes[k]; return !ok }):
			e.issues.add(CodeMatrixEntryInvalid, ep)
			e.ok = false
		}
		if shape == nil {
			shape = keys
		}
		for _, k := range keys {
			if !validMatrixKey(k) {
				e.issues.add(CodeMatrixKeyInvalid, joinPath(ep, k))
				e.ok = false
			} else if !validMatrixValue(entry[k]) {
				e.issues.add(CodeMatrixValueInvalid, joinPath(ep, k))
				e.ok = false
			}
		}
	}
}

func (e *matrixExpansion) product() []combination {
	if len(e.keys) == 0 {
		return nil
	}
	combos := []combination{{}}
	for _, k := range e.keys {
		next := make([]combination, 0, len(combos)*len(e.m.Axes[k]))
		for _, c := range combos {
			for _, v := range e.m.Axes[k] {
				nc := slices.Clone(c)
				next = append(next, append(nc, MatrixValue{Key: k, Value: v}))
			}
		}
		combos = next
	}
	return combos
}

func matches(c combination, entry map[string]Scalar) bool {
	for k, want := range entry {
		got, ok := c.get(k)
		if !ok || !sameScalar(got, want) {
			return false
		}
	}
	return true
}

func fromEntry(entry map[string]Scalar) combination {
	c := make(combination, 0, len(entry))
	for _, k := range sortedNames(entry) {
		c = append(c, MatrixValue{Key: k, Value: entry[k]})
	}
	return c
}

func (e *matrixExpansion) finish(combos []combination) ([]combination, bool) {
	switch {
	case len(combos) == 0:
		e.issues.add(CodeMatrixEmpty, e.path)
		return nil, false
	case len(combos) > MaxMatrixJobs:
		e.issues.add(CodeMatrixTooLarge, e.path)
		return nil, false
	}
	seen := make(map[string]bool, len(combos))
	for _, c := range combos {
		key := runJobKey("", c)
		if utf8.RuneCountInString(key) > MaxMatrixNameRunes {
			e.issues.add(CodeMatrixNameTooLong, e.path)
			return nil, false
		}
		if seen[key] {
			e.issues.add(CodeMatrixDuplicate, e.path)
			return nil, false
		}
		seen[key] = true
	}
	return combos, true
}

func matrixTypes(combos []combination) map[string]*cel.Type {
	kinds := map[string]ScalarKind{}
	for _, c := range combos {
		for _, v := range c {
			if k, ok := kinds[v.Key]; ok && k != v.Value.Kind {
				kinds[v.Key] = -1
				continue
			}
			kinds[v.Key] = v.Value.Kind
		}
	}
	out := make(map[string]*cel.Type, len(kinds))
	for k, kind := range kinds {
		switch kind {
		case KindString:
			out[k] = cel.StringType
		case KindInt:
			out[k] = cel.IntType
		case KindFloat:
			out[k] = cel.DoubleType
		case KindBool:
			out[k] = cel.BoolType
		default:
			out[k] = cel.DynType
		}
	}
	return out
}

package pipelinespec

import (
	"context"
	"reflect"
	"strconv"
	"strings"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"

	"github.com/Khaymah/dhow/internal/cachekey"
)

const hashMarker = "\x00"

var stringSliceType = reflect.TypeFor[[]string]()

type CacheKey struct {
	Template string
	Hashes   [][]string
}

type hashRecorder struct {
	calls [][]string
	bad   bool
}

func (h *hashRecorder) record(patterns []string) ref.Val {
	if len(h.calls) >= cachekey.MaxHashes || !cachekey.ValidPatterns(patterns) {
		h.bad = true
		return types.NewErr("hash_files takes 1 to %d relative patterns and is called at most %d times", cachekey.MaxPatterns, cachekey.MaxHashes)
	}
	h.calls = append(h.calls, patterns)
	return types.String(hashMarker + strconv.Itoa(len(h.calls)-1) + hashMarker)
}

func (h *hashRecorder) one(v ref.Val) ref.Val {
	s, ok := v.Value().(string)
	if !ok {
		return types.NewErr("hash_files takes text")
	}
	return h.record([]string{s})
}

func (h *hashRecorder) list(v ref.Val) ref.Val {
	raw, err := v.ConvertToNative(stringSliceType)
	if err != nil {
		return types.NewErr("hash_files takes a list of text")
	}
	patterns, _ := raw.([]string)
	return h.record(patterns)
}

func (h *hashRecorder) options() []cel.EnvOption {
	return []cel.EnvOption{cel.Function(hashFilesFunc,
		cel.Overload("hash_files_string", []*cel.Type{cel.StringType}, cel.StringType, cel.UnaryBinding(h.one)),
		cel.Overload("hash_files_list", []*cel.Type{cel.ListType(cel.StringType)}, cel.StringType, cel.UnaryBinding(h.list)),
	)}
}

func (r *RunJob) EvalCacheKey(ctx context.Context, c Context, env map[string]string) (CacheKey, error) {
	if r.Cache == nil {
		return CacheKey{}, ErrExpression
	}
	k := r.Cache.Key
	if k.Expression == "" {
		if !cachekey.ValidKey(k.Literal) {
			return CacheKey{}, ErrExpressionResult
		}
		return CacheKey{Template: cachekey.Escape(k.Literal)}, nil
	}
	c.Env = env
	rec := &hashRecorder{}
	out, err := r.evaluateWith(ctx, scopeCacheKey, k.Expression, c, rec.options())
	if err != nil {
		return CacheKey{}, err
	}
	if rec.bad {
		return CacheKey{}, ErrExpressionEval
	}
	s, ok := out.(string)
	if !ok {
		return CacheKey{}, ErrExpressionResult
	}
	return templateOf(s, rec.calls)
}

func templateOf(s string, calls [][]string) (CacheKey, error) {
	parts := strings.Split(s, hashMarker)
	if len(parts) != 2*len(calls)+1 {
		return CacheKey{}, ErrExpressionResult
	}
	var b cachekey.Builder
	seen := make([]bool, len(calls))
	for i, part := range parts {
		if i%2 == 0 {
			b.Text(part)
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 || n >= len(calls) || seen[n] || strconv.Itoa(n) != part {
			return CacheKey{}, ErrExpressionResult
		}
		seen[n] = true
		b.Hash(n)
	}
	t := b.String()
	if len(t) > cachekey.MaxTemplateBytes {
		return CacheKey{}, ErrExpressionResult
	}
	placeholder := make([]string, len(calls))
	for i := range placeholder {
		placeholder[i] = strings.Repeat("0", 64)
	}
	if _, err := cachekey.Render(t, placeholder); err != nil {
		return CacheKey{}, ErrExpressionResult
	}
	return CacheKey{Template: t, Hashes: calls}, nil
}

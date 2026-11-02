package executor

import (
	"testing"

	"github.com/stretchr/testify/assert"

	masking "github.com/Khaymah/dhow/apps/runner/internal/Masking"
)

func TestOutputsAreMaskedTogether(t *testing.T) {
	m := masking.New()
	m.Add([]byte("s3cr3t-value-123"))
	m.Add([]byte("abc"))
	tests := []struct {
		name string
		raw  map[string]string
		want map[string]string
	}{
		{"a whole secret in one value", map[string]string{"a": "x-s3cr3t-value-123", "b": "ok"}, map[string]string{"a": "x-***", "b": "ok"}},
		{"a secret split across keys in order", map[string]string{"a": "pre-s3cr3t-", "b": "value-123", "c": "fine"}, map[string]string{"c": "fine"}},
		{"a secret split across keys out of order", map[string]string{"a": "value-123", "b": "s3cr3t-", "c": "fine"}, map[string]string{"c": "fine"}},
		{"a secret split across three keys", map[string]string{"a": "s3cr", "b": "3t-val", "c": "ue-123", "d": "fine"}, map[string]string{"d": "fine"}},
		{"a secret shorter than MinLength is not masked", map[string]string{"a": "abc"}, map[string]string{"a": "abc"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, maskOutputs(m, tt.raw))
		})
	}
}

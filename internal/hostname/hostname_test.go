package hostname

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheck(t *testing.T) {
	tests := []struct {
		host string
		want error
	}{
		{"git.example.com", nil},
		{"xn--mgbh0fb.example", nil},
		{"localhost", nil},
		{"", ErrInvalid},
		{"Git.example.com", ErrInvalid},
		{"-x.example.com", ErrInvalid},
		{"a..b", ErrInvalid},
		{"10.0.0.1", ErrIP},
		{"[::1]", ErrIP},
		{"example.0x7f", ErrIP},
		{"example.123", ErrIP},
	}
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			assert.ErrorIs(t, Check(tt.host), tt.want)
			if tt.want == nil {
				assert.NoError(t, Check(tt.host))
			}
		})
	}
}

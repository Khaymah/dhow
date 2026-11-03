package pipelinespec_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Khaymah/dhow/internal/pipelinespec"
)

func TestValidEnvironmentName(t *testing.T) {
	for _, ok := range []string{"production", "staging-eu", "preview_1", "0", strings.Repeat("a", 64)} {
		assert.True(t, pipelinespec.ValidEnvironmentName(ok), "%q", ok)
	}
	for _, bad := range []string{"", "Production", "-prod", "_prod", "prod/eu", "prod eu", "prod.eu", strings.Repeat("a", 65), "../prod"} {
		assert.False(t, pipelinespec.ValidEnvironmentName(bad), "%q", bad)
	}
}

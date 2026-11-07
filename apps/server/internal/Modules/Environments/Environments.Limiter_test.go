package environments

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTheServiceRefusesToStartWithoutALimiter(t *testing.T) {
	s, err := NewService(Options{})
	require.ErrorIs(t, err, ErrNoLimiter)
	assert.Nil(t, s)
}

func TestADialIsRefusedWhenTheServiceHasNoLimiter(t *testing.T) {
	err := (&Service{}).chargeDial(context.Background(), uuid.New(), uuid.New())
	require.ErrorIs(t, err, ErrNoLimiter, "a missing limiter never lets a dial through")
}

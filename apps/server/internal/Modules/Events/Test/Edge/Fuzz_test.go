package edge_test

import (
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	events "github.com/Khaymah/dhow/apps/server/internal/Modules/Events"
)

func FuzzLastEventID(f *testing.F) {
	for _, seed := range []string{
		"", "1731000000-42", "0-0", "1-1", "-1-1", "1--1", "+1-1", "1-+1", "1-", "-1", "-", "--", "1-2-3", " 1-2", "1-2 ", "1\n-2",
		"9223372036854775807-18446744073709551615", "9223372036854775808-1", "1-18446744073709551616", "01-02", "0x1-0x2", "１-２",
		"1_0-2_0", "1e3-2", strings.Repeat("9", 400) + "-1", "1-" + strings.Repeat("9", 400), "\x00-\x00",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		pos, err := events.ParseLastEventID(raw)
		if raw == "" {
			require.NoError(t, err)
			require.Nil(t, pos, "no header means no position")
			return
		}
		if err != nil {
			require.Nil(t, pos)
			var app *apperrors.AppError
			require.ErrorAs(t, err, &app, "a bad id is a validation error, never a bare error")
			return
		}
		require.NotNil(t, pos)
		require.GreaterOrEqual(t, pos.Epoch, int64(0))
		require.LessOrEqual(t, pos.Epoch, int64(math.MaxInt64))
		again, err := events.ParseLastEventID(pos.ID())
		require.NoError(t, err, "the id the server writes is an id it reads")
		require.Equal(t, pos, again)
	})
}

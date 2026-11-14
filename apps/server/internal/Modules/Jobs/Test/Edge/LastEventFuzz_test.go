package edge_test

import (
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	jobs "github.com/Khaymah/dhow/apps/server/internal/Modules/Jobs"
)

func FuzzJobLogLastEventID(f *testing.F) {
	for _, seed := range []string{
		"1:0", "1:42", "2:9223372036854775806", "2:9223372036854775807", "0:1", "-1:1", "1:-1", "1:", ":1", ":", "1", "1:2:3", "+1:+2", " 1:2",
		"1:2 ", "2147483647:1", "2147483648:1", "01:02", "0x1:0x2", "１:２", strings.Repeat("9", 400) + ":1", "1:" + strings.Repeat("9", 400), "\x00:\x00",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		ref, err := jobs.ParseLastEvent(&raw)
		if err != nil {
			require.Nil(t, ref)
			var app *apperrors.AppError
			require.ErrorAs(t, err, &app, "a bad id is a validation error, never a bare error")
			return
		}
		require.NotNil(t, ref)
		require.GreaterOrEqual(t, ref.Attempt, 1)
		require.LessOrEqual(t, ref.Attempt, math.MaxInt32)
		require.GreaterOrEqual(t, ref.Seq, int64(0))
		require.Less(t, ref.Seq, int64(math.MaxInt64), "the next sequence after the id must not overflow")
		canonical := strconv.Itoa(ref.Attempt) + ":" + strconv.FormatInt(ref.Seq, 10)
		again, err := jobs.ParseLastEvent(&canonical)
		require.NoError(t, err)
		require.Equal(t, ref, again)
	})
}

func TestNoLastEventIDMeansNoPosition(t *testing.T) {
	ref, err := jobs.ParseLastEvent(nil)
	require.NoError(t, err)
	require.Nil(t, ref)
}

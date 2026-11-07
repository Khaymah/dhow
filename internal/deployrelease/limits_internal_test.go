package deployrelease

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOnlyTheLast4KiBOfTheRemoteAnswerIsLogged(t *testing.T) {
	const limit = 4 << 10
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"short", "done", "done"},
		{"exactly the limit", strings.Repeat("a", limit), strings.Repeat("a", limit)},
		{"one byte over keeps the end", "X" + strings.Repeat("b", limit), strings.Repeat("b", limit)},
		{"far over keeps the end", strings.Repeat("c", 10*limit) + "the end", strings.Repeat("c", limit-len("the end")) + "the end"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tail([]byte(tt.in)))
		})
	}
}

func TestAReportLineOf4KiBOrMoreIsDroppedWholeNotCut(t *testing.T) {
	var out bytes.Buffer
	r := NewStreamReporter(&out)
	fixed, err := jsonLen(Event{Event: EventHealth})
	require.NoError(t, err)

	r.Emit(Event{Event: EventHealth, Message: strings.Repeat("m", maxReportLine-fixed-1)})
	assert.Less(t, out.Len(), maxReportLine+1, "a line that fits is written")
	written := out.Len()
	require.Positive(t, written)

	r.Emit(Event{Event: EventHealth, Message: strings.Repeat("m", maxReportLine-fixed)})
	assert.Equal(t, written, out.Len(), "a line at the limit is not written, not even a cut part of it")
}

func TestTheReporterStopsOneEventBeforeTheServerLimit(t *testing.T) {
	var out bytes.Buffer
	r := NewStreamReporter(&out)
	for range MaxReportEvents * 2 {
		r.Emit(Event{Event: EventVerified})
	}
	assert.Equal(t, MaxReportEvents-1, bytes.Count(out.Bytes(), []byte("\n")))
}

func jsonLen(e Event) (int, error) {
	raw, err := json.Marshal(e)
	return len(raw), err
}

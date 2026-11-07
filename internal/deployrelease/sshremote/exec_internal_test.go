package sshremote

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func manySmallWrites() [][]byte {
	var out [][]byte
	for range 4 * maxOutput / 1000 {
		out = append(out, bytes.Repeat([]byte("b"), 1000))
	}
	return append(out, []byte("END"))
}

func TestTheRemoteOutputKeepsOnlyTheLast64KiBWhateverTheChunking(t *testing.T) {
	tests := []struct {
		name   string
		chunks [][]byte
	}{
		{"under the limit", [][]byte{[]byte("short answer")}},
		{"one huge write", [][]byte{append(bytes.Repeat([]byte("a"), 3*maxOutput), []byte("END")...)}},
		{"many small writes", manySmallWrites()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := &tailBuffer{}
			var all []byte
			for _, c := range tt.chunks {
				n, err := buf.Write(c)
				assert.NoError(t, err)
				assert.Equal(t, len(c), n, "the writer never reports a short write, or the session would fail")
				all = append(all, c...)
			}
			got := buf.bytes()
			assert.LessOrEqual(t, len(got), maxOutput)
			assert.Equal(t, all[max(0, len(all)-maxOutput):], got)
		})
	}
}

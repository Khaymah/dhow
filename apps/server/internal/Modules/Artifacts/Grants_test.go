package artifacts

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	blob "github.com/Khaymah/dhow/apps/server/internal/Core/Blob"
)

func TestAGrantOpensOneDigestForOneSubjectUntilItExpires(t *testing.T) {
	g := grants{key: []byte("0123456789abcdef0123456789abcdef")}
	now := time.Unix(1_800_000_000, 0)
	runJob, lease := uuid.New(), uuid.New()
	d := blob.Digest{1, 2, 3}
	other := blob.Digest{4}
	input := g.issue(grantInput, runJob, d, now.Add(time.Hour))
	cache := g.issue(grantCache, lease, d, now.Add(time.Minute))

	assert.True(t, g.verify(input, d, runJob, lease, now))
	assert.True(t, g.verify(cache, d, runJob, lease, now))
	cases := map[string]bool{
		"another digest":          g.verify(input, other, runJob, lease, now),
		"another run job":         g.verify(input, d, uuid.New(), lease, now),
		"a cache grant elsewhere": g.verify(cache, d, runJob, uuid.New(), now),
		"after it expired":        g.verify(input, d, runJob, lease, now.Add(time.Hour)),
		"another key":             grants{key: []byte("another key of thirty two bytes!")}.verify(input, d, runJob, lease, now),
		"a job without a run":     g.verify(input, d, uuid.Nil, lease, now),
		"garbage":                 g.verify("not-base64!", d, runJob, lease, now),
		"empty":                   g.verify("", d, runJob, lease, now),
	}
	for name, ok := range cases {
		assert.False(t, ok, name)
	}
	raw := []byte(input)
	raw[0] = 'C'
	assert.False(t, g.verify(string(raw), d, runJob, lease, now), "the kind is part of the MAC")
}

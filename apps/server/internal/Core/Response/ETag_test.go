package response_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	response "github.com/Khaymah/dhow/apps/server/internal/Core/Response"
)

type body struct {
	Name  string
	Count int
}

func TestTheETagIsStrongStableAndFollowsTheData(t *testing.T) {
	a, err := response.ETagOf(body{Name: "a", Count: 1})
	require.NoError(t, err)
	again, err := response.ETagOf(body{Name: "a", Count: 1})
	require.NoError(t, err)
	other, err := response.ETagOf(body{Name: "a", Count: 2})
	require.NoError(t, err)

	assert.Equal(t, a, again)
	assert.NotEqual(t, a, other)
	assert.Regexp(t, `^"[A-Za-z0-9_-]{22}"$`, a, "a quoted strong validator without a W/ prefix")
}

func TestAnETagThatCannotBeEncodedIsAnError(t *testing.T) {
	_, err := response.ETagOf(make(chan int))
	require.Error(t, err)
}

func TestIfNoneMatchMatchesTheETagInAnyOfTheForms(t *testing.T) {
	etag := `"abc"`
	list := `"zzz", "abc"`
	weak := `W/"abc"`
	star := `*`
	none := `"zzz"`
	empty := ``
	for _, tt := range []struct {
		name   string
		header *string
		want   bool
	}{
		{"the same value", &etag, true},
		{"one of a list", &list, true},
		{"a weak validator", &weak, true},
		{"any", &star, true},
		{"another value", &none, false},
		{"empty", &empty, false},
		{"no header", nil, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, response.ETagMatches(tt.header, etag))
		})
	}
	assert.False(t, response.ETagMatches(&star, ""), "nothing matches an empty ETag")
}

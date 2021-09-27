package fragment

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

var target, _ = url.Parse("http://fake.net")

func TestFragment_IntoRequestable(t *testing.T) {
	definition := Define("/hello/:name")
	requestable, err := definition.Requestable(
		target,
		map[string]string{":name": "fox.mulder"},
		url.Values{},
	)
	require.NoError(t, err)

	require.Equal(t, "http://fake.net/hello/fox.mulder", requestable.URL())
}

func TestFragment_HasDynamicParts(t *testing.T) {
	testCases := map[string]struct {
		input string
		want  bool
	}{
		"no dynamic parts": {input: "/foo/bar", want: false},
		"dynamic parts":    {input: "/:hello/namme", want: true},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			definition := Define(tc.input)
			require.Equal(t, tc.want, definition.HasDynamicParts())
		})
	}
}

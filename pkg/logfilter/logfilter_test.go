package logfilter

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLogFilter_FilterUrl(t *testing.T) {
	original, err := url.Parse("http://localhost/foo?a=1")
	require.NoError(t, err)

	filter := New()
	filtered := filter.FilterURL(original)

	require.Equal(t, "http://localhost/foo?a=1", original.String())
	require.Equal(t, "http://localhost/foo?a=FILTERED", filtered.String())
}

func TestLogFilter_FilterUrlUserInfo(t *testing.T) {
	original, err := url.Parse("http://foo:password@localhost/foo?a=1")
	require.NoError(t, err)

	filter := New()
	filtered := filter.FilterURL(original)

	require.Equal(t, "http://FILTERED:FILTERED@localhost/foo?a=FILTERED", filtered.String())
}

func TestLogFilter_FilterUrlString(t *testing.T) {
	tests := map[string]struct {
		input string
		allow []string
		want  string
	}{
		"no allowed parameters": {
			input: "http://localhost/foo/bar?a=1&b=2",
			allow: []string{},
			want:  "http://localhost/foo/bar?a=FILTERED&b=FILTERED",
		},
		"allowed param": {
			input: "http://localhost/foo/bar?a=1&b=2",
			allow: []string{"a"},
			want:  "http://localhost/foo/bar?a=1&b=FILTERED",
		},
		"path only url": {
			input: "/foo/bar?a=1&b=2",
			allow: []string{"a"},
			want:  "/foo/bar?a=1&b=FILTERED",
		},
		"mixed capitalization parameters": {
			input: "/foo/bar?A=1&b=2",
			allow: []string{"a"},
			want:  "/foo/bar?A=1&b=FILTERED",
		},
		"invalid url": {
			input: "http://%41:8080/",
			allow: []string{},
			want:  "FILTERED_INVALID_URL",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			filter := New()
			for _, value := range tc.allow {
				filter.Allow(value)
			}

			require.Equal(t, tc.want, filter.FilterURLString(tc.input))
		})
	}
}

func TestLogFilter_FilterQueryParams(t *testing.T) {
	tests := map[string]struct {
		input url.Values
		allow []string
		want  url.Values
	}{
		"no allowed params": {
			input: map[string][]string{"a": {"1"}, "b": {"2"}},
			allow: []string{},
			want:  map[string][]string{"a": {"FILTERED"}, "b": {"FILTERED"}},
		},

		"allowed params": {
			input: map[string][]string{"a": {"1"}, "b": {"2"}},
			allow: []string{"a", "b"},
			want:  map[string][]string{"a": {"1"}, "b": {"2"}},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			filter := New()
			for _, value := range tc.allow {
				filter.Allow(value)
			}

			require.Equal(t, tc.want, filter.FilterQueryParams(tc.input))
		})
	}
}

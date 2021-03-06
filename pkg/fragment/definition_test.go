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
	require.Equal(t, "http://fake.net/hello/:name", requestable.TemplateURL())
}

func TestFragment_IntoRequestable_MissingDynamicPart(t *testing.T) {
	definition := Define("/hello/:name")
	_, err := definition.Requestable(
		target,
		map[string]string{},
		url.Values{},
	)
	require.Error(t, err)
	require.EqualError(t, err, "no parameter was provided for :name in route /hello/:name")
}

func TestFragment_IntoRequestable_HandlesURLEncodings(t *testing.T) {
	definition := Define("/hello/:name")
	requestable, err := definition.Requestable(
		target,
		map[string]string{":name": "mulder%2fscully"},
		url.Values{},
	)
	require.NoError(t, err)
	require.Equal(t, "http://fake.net/hello/mulder%2fscully", requestable.URL())
	require.Equal(t, "http://fake.net/hello/:name", requestable.TemplateURL())
}

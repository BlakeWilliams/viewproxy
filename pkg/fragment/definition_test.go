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

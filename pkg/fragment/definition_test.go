package fragment

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

var target, _ = url.Parse("http://fake.net")

func TestFragment_InjectNamedParameters(t *testing.T) {
	definition := Define("/hello/:name")
	requestable, err := definition.Requestable(target, map[string]string{"name": "fox.mulder"})
	require.NoError(t, err)

	require.Equal(t, "http://fake.net/hello/fox.mulder", requestable.URL())
}


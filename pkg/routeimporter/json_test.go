package routeimporter

import (
	"io/ioutil"
	"log"
	"os"
	"testing"

	"github.com/blakewilliams/viewproxy"
	"github.com/stretchr/testify/require"
)

func TestLoadJSONFile(t *testing.T) {
	viewproxyServer := viewproxy.NewServer("http://fake.net")
	viewproxyServer.Logger = log.New(ioutil.Discard, "", log.Ldate|log.Ltime)

	// Load routes from config
	file, err := ioutil.TempFile(os.TempDir(), "config.json")
	if err != nil {
		t.Error(err)
	}
	defer os.Remove(file.Name())

	file.Write([]byte(`[
		{
			"url": "/users/new",
			"metadata": {
				"controller": "sessions"
			},
			"layout": {
				"path": "/_viewproxy/users/new/layout"
			},
			"fragments": [
				{
					"path": "/_viewproxy/users/new/content"
				}
			]
		}
	]`))

	file.Close()

	LoadJSONFile(viewproxyServer, file.Name())

	routes := viewproxyServer.Routes()
	require.Len(t, routes, 1)
	route := routes[0]

	require.Equal(t, "/users/new", route.Path)
	require.Equal(t, "sessions", route.Metadata["controller"])
	require.Equal(t, "/_viewproxy/users/new/layout", route.LayoutFragment.Path)
	require.Len(t, route.ContentFragments, 1)
	require.Equal(t, "/_viewproxy/users/new/content", route.ContentFragments[0].Path)
}

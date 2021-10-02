package routeimporter

import (
	"testing"

	"github.com/blakewilliams/viewproxy"
	"github.com/stretchr/testify/require"
)

func TestLoadRoutesError(t *testing.T) {
	server, err := viewproxy.NewServer("localhost:9999")
	require.NoError(t, err)

	entry := ConfigRouteEntry{
		Url:  "/foo/bar",
		Root: ConfigFragment{Path: "/layout/:name"},
	}

	err = LoadRoutes(server, []ConfigRouteEntry{entry})
	require.Error(t, err)
}

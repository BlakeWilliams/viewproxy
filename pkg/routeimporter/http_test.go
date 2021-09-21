package routeimporter

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/blakewilliams/viewproxy"
	"github.com/stretchr/testify/require"
)

var jsonConfig = []byte(`[
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
]`)

func TestLoadHttp(t *testing.T) {
	targetServer := startTargetServer()
	defer targetServer.CloseClientConnections()
	defer targetServer.Close()

	viewproxyServer := viewproxy.NewServer(targetServer.URL)

	err := LoadHttp(viewproxyServer, "/_viewproxy_routes")
	require.NoError(t, err)
	viewproxyServer.Logger = log.New(ioutil.Discard, "", log.Ldate|log.Ltime)

	requireJsonConfigRoutesLoaded(t, viewproxyServer.Routes())
}

func TestLoadHttp_HMAC(t *testing.T) {
	hmacSecret := "abc123"

	instance := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization := r.Header.Get("Authorization")
		timestamp := r.Header.Get("X-Authorization-Time")

		require.NotEmpty(t, authorization)
		require.NotEmpty(t, timestamp)

		mac := hmac.New(sha256.New, []byte(hmacSecret))
		mac.Write(
			[]byte(fmt.Sprintf("%s,%s", r.URL.Path, timestamp)),
		)

		require.Equal(t, hex.EncodeToString(mac.Sum(nil)), authorization)

		w.Write(jsonConfig)
	})

	testServer := httptest.NewServer(instance)
	defer testServer.CloseClientConnections()
	defer testServer.Close()

	viewproxyServer := viewproxy.NewServer(testServer.URL)
	viewproxyServer.HmacSecret = hmacSecret

	err := LoadHttp(viewproxyServer, "/_viewproxy_routes")
	require.NoError(t, err)
	viewproxyServer.Logger = log.New(ioutil.Discard, "", log.Ldate|log.Ltime)

	requireJsonConfigRoutesLoaded(t, viewproxyServer.Routes())
}

func startTargetServer() *httptest.Server {
	instance := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/_viewproxy_routes" {
			w.Header().Set("Content-Type", "text/json")
			w.WriteHeader(http.StatusOK)

			w.Write(jsonConfig)
		} else {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("target: 404 not found"))
		}
	})

	testServer := httptest.NewServer(instance)
	return testServer
}

func requireJsonConfigRoutesLoaded(t *testing.T, routes []viewproxy.Route) {
	require.Len(t, routes, 1)
	route := routes[0]

	require.Equal(t, "/users/new", route.Path)
	require.Equal(t, "sessions", route.Metadata["controller"])
	require.Equal(t, "/_viewproxy/users/new/layout", route.LayoutFragment.Path)
	require.Len(t, route.ContentFragments, 1)
	require.Equal(t, "/_viewproxy/users/new/content", route.ContentFragments[0].Path)
}
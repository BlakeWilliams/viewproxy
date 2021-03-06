package logging

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/blakewilliams/viewproxy"
	"github.com/blakewilliams/viewproxy/pkg/fragment"
	"github.com/blakewilliams/viewproxy/pkg/multiplexer"
	"github.com/blakewilliams/viewproxy/pkg/secretfilter"
	"github.com/stretchr/testify/require"
)

type SliceLogger struct {
	logs []string
	mu   sync.Mutex
}

func (l *SliceLogger) Print(v ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logs = append(l.logs, fmt.Sprint(v...))
}

func (l *SliceLogger) Printf(line string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logs = append(l.logs, fmt.Sprintf(line, args...))
}

func TestLoggingMiddleware(t *testing.T) {
	targetServer := startTargetServer()
	viewProxyServer, err := viewproxy.NewServer(targetServer.URL)
	require.NoError(t, err)

	viewProxyServer.Get(
		"/hello/:name",
		fragment.Define(
			"/layouts/test_layout/:name",
			fragment.WithChild("body", fragment.Define("/body/:name")),
		),
	)

	log := &SliceLogger{logs: make([]string, 0)}
	viewProxyServer.AroundRequest = func(handler http.Handler) http.Handler {
		handler = Middleware(viewProxyServer, log)(handler)

		return handler
	}

	// Regular request with fragments
	r := httptest.NewRequest("GET", "/hello/world", nil)
	w := httptest.NewRecorder()
	viewProxyServer.CreateHandler().ServeHTTP(w, r)
	resp := w.Result()
	require.Equal(t, 200, resp.StatusCode)

	require.Equal(t, "Handling /hello/world", log.logs[0])
	require.Regexp(t, regexp.MustCompile(`Rendered 200 in \d+ms for /hello/world`), log.logs[1])

	// Proxying disabled
	r = httptest.NewRequest("GET", "/fake", nil)
	w = httptest.NewRecorder()
	viewProxyServer.CreateHandler().ServeHTTP(w, r)
	resp = w.Result()
	require.Equal(t, 404, resp.StatusCode)

	require.Equal(t, "Proxying is disabled and no route matches /fake", log.logs[2])
}

func TestLogTripperFragments(t *testing.T) {
	targetServer := startTargetServer()
	viewProxyServer, err := viewproxy.NewServer(targetServer.URL, viewproxy.WithPassThrough(targetServer.URL))
	require.NoError(t, err)

	viewProxyServer.Get(
		"/hello/:name",
		fragment.Define("/layouts/test_layout/:name", fragment.WithChild("body", fragment.Define("/body/:name"))),
	)

	log := &SliceLogger{logs: make([]string, 0)}
	viewProxyServer.MultiplexerTripper = NewLogTripper(log, secretfilter.New(), multiplexer.NewStandardTripper(&http.Client{}))

	r := httptest.NewRequest("GET", "/hello/world", nil)
	w := httptest.NewRecorder()
	viewProxyServer.CreateHandler().ServeHTTP(w, r)
	resp := w.Result()
	require.Equal(t, 200, resp.StatusCode)

	require.Regexp(t, regexp.MustCompile(`Fragment 200 in \d+ms for http:\/\/.*`), log.logs[0])
	require.Regexp(t, regexp.MustCompile(`Fragment 200 in \d+ms for http:\/\/.*`), log.logs[1])
}

func startTargetServer() *httptest.Server {
	instance := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(r.URL.Path, "/")
		name := parts[len(parts)-1]

		if strings.HasPrefix(r.URL.Path, "/layouts/test_layout/") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("<html><view-proxy-content></view-proxy-content></html>"))
		} else if strings.HasPrefix(r.URL.Path, "/header") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("<body>"))
		} else if strings.HasPrefix(r.URL.Path, "/body/") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(fmt.Sprintf("hello %s", name)))
		} else {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("404 not found"))
		}
	})

	testServer := httptest.NewServer(instance)
	return testServer
}

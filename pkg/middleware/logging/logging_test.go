package logging

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/blakewilliams/viewproxy"
	"github.com/blakewilliams/viewproxy/pkg/multiplexer"
	"github.com/blakewilliams/viewproxy/pkg/secretfilter"
	"github.com/stretchr/testify/assert"
)

type SliceLogger struct {
	logs []string
}

func (l *SliceLogger) Print(v ...interface{}) {
	l.logs = append(l.logs, fmt.Sprint(v...))
}

func (l *SliceLogger) Printf(line string, args ...interface{}) {
	l.logs = append(l.logs, fmt.Sprintf(line, args...))
}

func TestLoggingMiddleware(t *testing.T) {
	targetServer := startTargetServer()
	viewProxyServer := viewproxy.NewServer(targetServer.URL)
	viewProxyServer.PassThrough = true

	layout := viewproxy.NewFragment("/layouts/test_layout")
	fragments := []*viewproxy.FragmentRoute{
		viewproxy.NewFragment("body"),
	}
	viewProxyServer.Get("/hello/:name", layout, fragments)

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
	assert.Equal(t, 200, resp.StatusCode)

	assert.Equal(t, "Handling /hello/world", log.logs[0])
	assert.Regexp(t, regexp.MustCompile(`Rendered 200 in \d+ms for /hello/world`), log.logs[1])

	// Proxying request
	r = httptest.NewRequest("GET", "/fake", nil)
	w = httptest.NewRecorder()
	viewProxyServer.CreateHandler().ServeHTTP(w, r)
	resp = w.Result()
	assert.Equal(t, 404, resp.StatusCode)

	assert.Equal(t, "Proxying /fake", log.logs[2])
	assert.Regexp(t, regexp.MustCompile(`Proxied 404 in \d+ms for /fake`), log.logs[3])

	// Proxying disabled
	viewProxyServer.PassThrough = false
	r = httptest.NewRequest("GET", "/fake", nil)
	w = httptest.NewRecorder()
	viewProxyServer.CreateHandler().ServeHTTP(w, r)
	resp = w.Result()
	assert.Equal(t, 404, resp.StatusCode)

	assert.Equal(t, "Proxying is disabled and no route matches /fake", log.logs[4])
}

func TestLogTripperFragments(t *testing.T) {
	targetServer := startTargetServer()
	viewProxyServer := viewproxy.NewServer(targetServer.URL)
	viewProxyServer.PassThrough = true

	layout := viewproxy.NewFragment("/layouts/test_layout")
	fragments := []*viewproxy.FragmentRoute{
		viewproxy.NewFragment("body"),
	}
	viewProxyServer.Get("/hello/:name", layout, fragments)

	log := &SliceLogger{logs: make([]string, 0)}
	viewProxyServer.MultiplexerTripper = NewLogTripper(log, secretfilter.New(), multiplexer.NewStandardTripper(&http.Client{}))

	r := httptest.NewRequest("GET", "/hello/world", nil)
	w := httptest.NewRecorder()
	viewProxyServer.CreateHandler().ServeHTTP(w, r)
	resp := w.Result()
	assert.Equal(t, 200, resp.StatusCode)

	fmt.Println(log.logs)

	assert.Regexp(t, regexp.MustCompile(`Fragment 200 in \d+ms for http:\/\/.*`), log.logs[0])
	assert.Regexp(t, regexp.MustCompile(`Fragment 200 in \d+ms for http:\/\/.*`), log.logs[1])
}

func TestLogTripperProxy(t *testing.T) {
	targetServer := startTargetServer()
	viewProxyServer := viewproxy.NewServer(targetServer.URL)
	viewProxyServer.PassThrough = true

	layout := viewproxy.NewFragment("/layouts/test_layout")
	fragments := []*viewproxy.FragmentRoute{
		viewproxy.NewFragment("body"),
	}
	viewProxyServer.Get("/hello/:name", layout, fragments)

	log := &SliceLogger{logs: make([]string, 0)}
	viewProxyServer.MultiplexerTripper = NewLogTripper(log, secretfilter.New(), multiplexer.NewStandardTripper(&http.Client{}))

	r := httptest.NewRequest("GET", "/fake", nil)
	w := httptest.NewRecorder()
	viewProxyServer.CreateHandler().ServeHTTP(w, r)
	resp := w.Result()
	assert.Equal(t, 404, resp.StatusCode)

	fmt.Println(log.logs)
	assert.Regexp(t, regexp.MustCompile(`Proxy request 404 in \d+ms for http:\/\/.*`), log.logs[0])
}

func startTargetServer() *httptest.Server {
	instance := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		params := r.URL.Query()

		if r.URL.Path == "/layouts/test_layout" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("<html><view-proxy-content></view-proxy-content></html>"))
		} else if r.URL.Path == "/header" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("<body>"))
		} else if r.URL.Path == "/body" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(fmt.Sprintf("hello %s", params.Get("name"))))
		} else if r.URL.Path == "/boom" {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(fmt.Sprintf("hello %s", params.Get("name"))))
		} else {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("404 not found"))
		}
	})

	testServer := httptest.NewServer(instance)
	return testServer
}

package viewproxy

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blakewilliams/viewproxy/pkg/fragment"
	"github.com/blakewilliams/viewproxy/pkg/multiplexer"
	"github.com/stretchr/testify/require"
)

var legacyTargetServer *httptest.Server
var targetServer *httptest.Server

func TestMain(m *testing.M) {
	legacyTargetServer = startLegacyTargetServer()
	defer legacyTargetServer.CloseClientConnections()
	defer legacyTargetServer.Close()

	targetServer = startTargetServer()
	defer targetServer.CloseClientConnections()
	defer targetServer.Close()

	os.Exit(m.Run())
}

func TestServer(t *testing.T) {
	viewProxyServer := newServer(t, targetServer.URL)
	viewProxyServer.Addr = "localhost:9997"
	viewProxyServer.Logger = log.New(ioutil.Discard, "", log.Ldate|log.Ltime)

	viewProxyServer.AroundResponse = func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			rw.Header().Del("etag")
			next.ServeHTTP(rw, r)
		})
	}

	// layout is shared and has no :name fragment
	layout := fragment.Define("/layouts/test_layout", fragment.WithoutValidation())

	fragments := fragment.Collection{
		fragment.Define("/header/:name"),
		fragment.Define("/body/:name"),
		fragment.Define("/footer/:name"),
	}
	err := viewProxyServer.Get("/hello/:name", layout, fragments)
	require.NoError(t, err)
	viewProxyServer.Logger = log.New(os.Stdout, "", log.Ldate|log.Ltime)

	go func() {
		if err := viewProxyServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()

	resp, err := http.Get(fmt.Sprintf("http://localhost:9997%s", "/hello/world"))
	require.NoError(t, err)
	body, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, "<html><body>hello world</body></html>", string(body))
}

func TestServer_LegacyRoutes(t *testing.T) {
	viewProxyServer := newServer(t, legacyTargetServer.URL)
	viewProxyServer.Addr = "localhost:9998"
	viewProxyServer.Logger = log.New(ioutil.Discard, "", log.Ldate|log.Ltime)

	viewProxyServer.AroundResponse = func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			rw.Header().Del("etag")
			next.ServeHTTP(rw, r)
		})
	}

	layout := fragment.Define("/layouts/test_layout")
	fragments := fragment.Collection{
		fragment.Define("/header", fragment.WithMetadata(map[string]string{"legacy": "true"})),
		fragment.Define("/body", fragment.WithMetadata(map[string]string{"legacy": "true"})),
		fragment.Define("/footer", fragment.WithMetadata(map[string]string{"legacy": "true"})),
	}
	viewProxyServer.Get("/hello/:name", layout, fragments, WithRouteMetadata(map[string]string{"legacy": "true"}))
	viewProxyServer.Logger = log.New(os.Stdout, "", log.Ldate|log.Ltime)

	go func() {
		if err := viewProxyServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()

	resp, err := http.Get(fmt.Sprintf("http://localhost:9998%s", "/hello/world"))
	require.NoError(t, err)
	body, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, "<html><body>hello world</body></html>", string(body))
	require.Equal(t, "viewproxy", resp.Header.Get("x-name"), "Expected response to have an X-Name header")
	require.Equal(t, "", resp.Header.Get("etag"), "Expected response to have removed etag header")
}

func TestQueryParamForwardingServer(t *testing.T) {
	viewProxyServer := newServer(t, legacyTargetServer.URL)
	viewProxyServer.Logger = log.New(ioutil.Discard, "", log.Ldate|log.Ltime)

	layout := fragment.Define("/layouts/test_layout")
	fragments := fragment.Collection{
		fragment.Define("header", fragment.WithMetadata(map[string]string{"legacy": "true"})),
		fragment.Define("body", fragment.WithMetadata(map[string]string{"legacy": "true"})),
		fragment.Define("footer", fragment.WithMetadata(map[string]string{"legacy": "true"})),
	}
	viewProxyServer.Get("/hello/:name", layout, fragments, WithRouteMetadata(map[string]string{"legacy": "true"}))

	r := httptest.NewRequest("GET", "/hello/world?important=true&name=override", nil)
	w := httptest.NewRecorder()

	viewProxyServer.CreateHandler().ServeHTTP(w, r)

	resp := w.Result()

	body, err := ioutil.ReadAll(resp.Body)
	require.Nil(t, err)
	expected := "<html><body>hello world!</body></html>"

	require.Equal(t, expected, string(body))
	require.Equal(t, "viewproxy", resp.Header.Get("x-name"), "Expected response to have an X-Name header")
}

func TestPassThroughEnabled(t *testing.T) {
	viewProxyServer := newServer(t, legacyTargetServer.URL, WithPassThrough(legacyTargetServer.URL))
	viewProxyServer.Logger = log.New(ioutil.Discard, "", log.Ldate|log.Ltime)

	r := httptest.NewRequest("GET", "/oops", nil)
	w := httptest.NewRecorder()

	viewProxyServer.CreateHandler().ServeHTTP(w, r)

	resp := w.Result()
	body, err := ioutil.ReadAll(resp.Body)
	require.Nil(t, err)

	require.Equal(t, 500, resp.StatusCode)
	require.Equal(t, "Something went wrong", string(body))
}

func TestPassThroughDisabled(t *testing.T) {
	viewProxyServer := newServer(t, legacyTargetServer.URL)

	r := httptest.NewRequest("GET", "/hello/world", nil)
	w := httptest.NewRecorder()

	viewProxyServer.CreateHandler().ServeHTTP(w, r)

	resp := w.Result()
	body, err := ioutil.ReadAll(resp.Body)
	require.Nil(t, err)

	require.Equal(t, 404, resp.StatusCode)
	require.Equal(t, "404 not found", string(body))
}

func TestPassThroughPostRequest(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	done := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(done)

		body, err := io.ReadAll(r.Body)

		require.Nil(t, err)
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "hello", string(body))
	}))

	viewProxyServer := newServer(t, server.URL, WithPassThrough(server.URL))

	r := httptest.NewRequest("POST", "/hello/world", strings.NewReader("hello"))
	w := httptest.NewRecorder()

	viewProxyServer.CreateHandler().ServeHTTP(w, r)

	select {
	case <-done:
		server.Close()
	case <-ctx.Done():
		require.Fail(t, ctx.Err().Error())
	}
}

func TestFragmentSendsVerifiableHmacWhenSet(t *testing.T) {
	done := make(chan struct{})
	secret := "6ccd9547b7042e0f1101ce68931d6b2c"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(done)

		time := r.Header.Get("X-Authorization-Time")
		require.NotEqual(t, "", time, "Expected X-Authorization-Time header to be present")

		key := fmt.Sprintf("%s,%s", r.URL.Path, time)

		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(
			[]byte(key),
		)

		authorization := r.Header.Get("Authorization")
		require.NotEqual(t, "", authorization, "Expected Authorization header to be present")

		expected := hex.EncodeToString(mac.Sum(nil))

		require.Equal(t, expected, authorization)

		w.WriteHeader(http.StatusOK)
	}))

	viewProxyServer := newServer(t, server.URL)
	viewProxyServer.Get("/hello/:name", fragment.Define("/foo"), fragment.Collection{}, WithRouteMetadata(map[string]string{"legacy": "true"}))
	viewProxyServer.HmacSecret = secret

	r := httptest.NewRequest("GET", "/hello/world", strings.NewReader("hello"))
	w := httptest.NewRecorder()

	viewProxyServer.CreateHandler().ServeHTTP(w, r)

	<-done

	server.Close()
}

func TestFragmentSetsCorrectHeaders(t *testing.T) {
	layoutDone := make(chan bool)
	fragmentDone := make(chan bool)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/foo" {
			defer close(layoutDone)
			w.Header().Set("Server-Timing", "db;dur=12, git;dur=0")
		} else if r.URL.Path == "/bar" {
			defer close(fragmentDone)
			w.Header().Set("Server-Timing", "db;dur=34")
		}
		require.Equal(t, r.Header.Get(HeaderViewProxyOriginalPath), "/hello/world?foo=bar")
		require.Equal(t, "", r.Header.Get("Keep-Alive"), "Expected Keep-Alive to be filtered")
		require.NotEqual(t, "", r.Header.Get("X-Forwarded-For"))
		require.Equal(t, "localhost:1", r.Header.Get("X-Forwarded-Host"))
		w.WriteHeader(http.StatusOK)
	}))

	viewProxyServer := newServer(t, server.URL)
	layout := fragment.Define("/foo", fragment.WithTimingLabel("foo"))
	content := fragment.Define("/bar", fragment.WithTimingLabel("bar"))
	viewProxyServer.Get("/hello/:name", layout, fragment.Collection{content}, WithRouteMetadata(map[string]string{"legacy": "true"}))

	r := httptest.NewRequest("GET", "/hello/world?foo=bar", strings.NewReader("hello"))
	r.Host = "localhost:1" // go deletes the Host header and sets the Host field
	r.RemoteAddr = "localhost:1"
	r.Header.Add(HeaderViewProxyOriginalPath, "/fake/path")
	w := httptest.NewRecorder()

	viewProxyServer.CreateHandler().ServeHTTP(w, r)

	<-layoutDone
	<-fragmentDone

	resp := w.Result()

	require.Contains(t, resp.Header.Get("Server-Timing"), "foo-db;desc=\"foo db\";dur=12")
	require.Contains(t, resp.Header.Get("Server-Timing"), "bar-db;desc=\"bar db\";dur=34")
	require.Contains(t, resp.Header.Get("Server-Timing"), "foo-fragment;desc=\"foo fragment\";dur=")
	require.Contains(t, resp.Header.Get("Server-Timing"), "bar-fragment;desc=\"bar fragment\";dur=")

	server.Close()
}

func TestSupportsGzip(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b bytes.Buffer

		gzWriter := gzip.NewWriter(&b)

		if r.URL.Path == "/layout" {
			gzWriter.Write([]byte("<body><view-proxy-content></view-proxy-content></body>"))
		} else if r.URL.Path == "/fragment" {
			gzWriter.Write([]byte("wow gzipped!"))
		} else {
			panic("Unexpected URL")
		}

		gzWriter.Close()

		w.Header().Set("Content-Encoding", "gzip")
		w.WriteHeader(http.StatusOK)
		w.Write(b.Bytes())
	}))

	viewProxyServer := newServer(t, server.URL)
	viewProxyServer.Get("/hello/:name", fragment.Define("/layout"), fragment.Collection{fragment.Define("/fragment")}, WithRouteMetadata(map[string]string{"legacy": "true"}))

	r := httptest.NewRequest("GET", "/hello/world", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	viewProxyServer.CreateHandler().ServeHTTP(w, r)

	resp := w.Result()

	gzReader, err := gzip.NewReader(resp.Body)
	require.Nil(t, err)

	body, err := ioutil.ReadAll(gzReader)
	require.Nil(t, err)

	require.Equal(t, "<body>wow gzipped!</body>", string(body))

	server.Close()
}

func TestAroundRequestCallback(t *testing.T) {
	done := make(chan struct{})

	server := newServer(t, targetServer.URL)
	server.Get("/hello/:name", fragment.Define("/layout"), fragment.Collection{fragment.Define("/fragment")}, WithRouteMetadata(map[string]string{"legacy": "true"}))
	server.AroundRequest = func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer close(done)
			w.Header().Set("x-viewproxy", "true")
			require.Equal(t, "/hello/:name", RouteFromContext(r.Context()).Path)
			require.Equal(t, "192.168.1.1", r.RemoteAddr)
			next.ServeHTTP(w, r)
		})
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/hello/world", nil)
	r.RemoteAddr = "192.168.1.1"

	server.CreateHandler().ServeHTTP(w, r)

	resp := w.Result()

	require.Equal(t, "true", resp.Header.Get("x-viewproxy"))

	<-done
}

func TestErrorHandler(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	done := make(chan struct{})

	server := newServer(t, legacyTargetServer.URL)
	server.Get(
		"/hello/:name",
		fragment.Define("/definitely_missing_and_not_defined", fragment.WithMetadata(map[string]string{"legacy": "true"})),
		fragment.Collection{},
		WithRouteMetadata(map[string]string{"legacy": "true"}),
	)
	server.AroundRequest = func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("x-viewproxy", "true")
			require.Equal(t, "192.168.1.1", r.RemoteAddr)
			next.ServeHTTP(w, r)
		})
	}
	server.AroundResponse = func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			rw.Header().Set("error-header", "true")

			defer close(done)

			results := multiplexer.ResultsFromContext(r.Context())
			require.NotNil(t, results)

			var resultErr *ResultError
			require.ErrorAs(t, results.Error(), &resultErr)
			require.Equal(
				t,
				fmt.Sprintf("%s/definitely_missing_and_not_defined?name=world", legacyTargetServer.URL),
				resultErr.Result.Url,
			)
			require.Equal(t, 404, resultErr.Result.StatusCode)
		})
	}

	fakeWriter := httptest.NewRecorder()
	fakeRequest := httptest.NewRequest("GET", "/hello/world", nil)
	fakeRequest.RemoteAddr = "192.168.1.1"

	server.CreateHandler().ServeHTTP(fakeWriter, fakeRequest)

	require.Equal(t, "true", fakeWriter.Header().Get("x-viewproxy"))
	require.Equal(t, "true", fakeWriter.Header().Get("error-header"))

	select {
	case <-done:
	case <-ctx.Done():
		require.Fail(t, ctx.Err().Error())
	}
}

type contextTestTripper struct {
	route        *Route
	requestables []multiplexer.Requestable
	mu           sync.Mutex
}

func (t *contextTestTripper) Request(r *http.Request) (*http.Response, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.route = RouteFromContext(r.Context())
	t.requestables = append(t.requestables, multiplexer.RequestableFromContext(r.Context()))
	return http.DefaultClient.Do(r)
}

func TestRoundTripperContext(t *testing.T) {
	viewProxyServer, err := NewServer(legacyTargetServer.URL)
	require.NoError(t, err)
	viewProxyServer.Logger = log.New(ioutil.Discard, "", log.Ldate|log.Ltime)
	tripper := &contextTestTripper{}
	viewProxyServer.MultiplexerTripper = tripper

	layout := fragment.Define("/layouts/test_layout")
	routeFragments := fragment.Collection{
		fragment.Define("header"),
		fragment.Define("body"),
		fragment.Define("footer"),
	}
	viewProxyServer.Get("/hello/:name", layout, routeFragments, WithRouteMetadata(map[string]string{"legacy": "true"}))

	r := httptest.NewRequest("GET", "/hello/world?important=true&name=override", nil)
	w := httptest.NewRecorder()

	viewProxyServer.CreateHandler().ServeHTTP(w, r)

	resp := w.Result()

	require.Equal(t, 200, resp.StatusCode)
	require.Equal(t, 4, len(tripper.requestables))
	require.NotNil(t, tripper.route)
}

func TestWithPassThrough_Error(t *testing.T) {
	_, err := NewServer(legacyTargetServer.URL, WithPassThrough("%invalid%"))

	require.Error(t, err)
	require.Contains(t, err.Error(), "viewproxy.ServerOption error")
	require.Contains(t, err.Error(), "WithPassThrough error")
}

func startLegacyTargetServer() *httptest.Server {
	instance := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		params := r.URL.Query()

		w.Header().Set("EtAg", "1234")
		w.Header().Set("X-Name", "viewproxy")

		if r.URL.Path == "/layouts/test_layout" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("<html><view-proxy-content></view-proxy-content></html>"))
		} else if r.URL.Path == "/header" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("<body>"))
		} else if r.URL.Path == "/body" {
			w.WriteHeader(http.StatusOK)
			if params.Get("important") != "" {
				w.Write([]byte(fmt.Sprintf("hello %s!", params.Get("name"))))
			} else {
				w.Write([]byte(fmt.Sprintf("hello %s", params.Get("name"))))
			}
		} else if r.URL.Path == "/footer" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("</body>"))
		} else if r.URL.Path == "/oops" {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Something went wrong"))
		} else {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("target: 404 not found"))
		}
	})

	testServer := httptest.NewServer(instance)
	return testServer
}

func startTargetServer() *httptest.Server {
	instance := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(r.URL.Path, "/")
		name := parts[len(parts)-1]

		if r.URL.Path == "/layouts/test_layout" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("<html><view-proxy-content></view-proxy-content></html>"))
		} else if strings.HasPrefix(r.URL.Path, "/header/") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("<body>"))
		} else if strings.HasPrefix(r.URL.Path, "/body/") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(fmt.Sprintf("hello %s", name)))
		} else if strings.HasPrefix(r.URL.Path, "/footer/") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("</body>"))
		} else {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("target: 404 not found"))
		}
	})

	testServer := httptest.NewServer(instance)
	return testServer
}

func newServer(t *testing.T, target string, opts ...ServerOption) *Server {
	server, err := NewServer(target, opts...)
	require.NoError(t, err)

	return server
}

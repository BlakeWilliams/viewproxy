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
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blakewilliams/viewproxy/pkg/fragment"
	"github.com/blakewilliams/viewproxy/pkg/multiplexer"
	"github.com/stretchr/testify/require"
)

var targetServer *httptest.Server

func TestMain(m *testing.M) {
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
	root := fragment.Define(
		"/layouts/test_layout", fragment.WithoutValidation(),
		fragment.WithChild("header", fragment.Define("/header/:name")),
		fragment.WithChild("body", fragment.Define("/body/:name")),
		fragment.WithChild("footer", fragment.Define("/footer/:name")),
	)

	err := viewProxyServer.Get("/hello/:name", root)
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

func TestQueryParamForwardingServer(t *testing.T) {
	viewProxyServer := newServer(t, targetServer.URL)
	viewProxyServer.Logger = log.New(ioutil.Discard, "", log.Ldate|log.Ltime)

	root := fragment.Define("/layouts/test_layout",
		fragment.WithoutValidation(),
		fragment.WithChild("header", fragment.Define("/header/:name")),
		fragment.WithChild("body", fragment.Define("/body/:name")),
		fragment.WithChild("footer", fragment.Define("/footer/:name")),
	)
	viewProxyServer.Get("/hello/:name", root)

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

func TestServer_EscapedNamedFragments(t *testing.T) {
	viewProxyServer := newServer(t, targetServer.URL)

	root := fragment.Define("/layouts/test_layout",
		fragment.WithoutValidation(),
		fragment.WithChild("header", fragment.Define("/header/:name")),
		fragment.WithChild("body", fragment.Define("/body/:name")),
		fragment.WithChild("footer", fragment.Define("/footer/:name")),
	)
	err := viewProxyServer.Get("/hello/:name", root)
	require.NoError(t, err)

	r := httptest.NewRequest("GET", "/hello/world%2fvoltron", nil)
	w := httptest.NewRecorder()

	viewProxyServer.CreateHandler().ServeHTTP(w, r)

	resp := w.Result()

	body, err := ioutil.ReadAll(resp.Body)
	require.Nil(t, err)
	expected := "<html><body>hello world/voltron</body></html>"

	require.Equal(t, expected, string(body))
}

func TestPassThroughEnabled(t *testing.T) {
	viewProxyServer := newServer(t, targetServer.URL, WithPassThrough(targetServer.URL))
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
	viewProxyServer := newServer(t, targetServer.URL)

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
	err := viewProxyServer.Get("/hello/:name", fragment.Define("/foo/:name"))
	require.NoError(t, err)
	viewProxyServer.HmacSecret = secret

	r := httptest.NewRequest("GET", "/hello/world", strings.NewReader("hello"))
	w := httptest.NewRecorder()

	viewProxyServer.CreateHandler().ServeHTTP(w, r)

	<-done

	server.Close()
}

func TestSupportsGzip(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b bytes.Buffer

		gzWriter := gzip.NewWriter(&b)

		if strings.HasPrefix(r.URL.Path, "/layout") {
			gzWriter.Write([]byte(`<body><viewproxy-fragment id="fragment"></viewproxy-fragment></body>`))
		} else if strings.HasPrefix(r.URL.Path, "/fragment") {
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
	viewProxyServer.Get(
		"/hello/:name",
		fragment.Define("/layout/:name", fragment.WithChild("fragment", fragment.Define("/fragment/:name"))),
	)

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
	err := server.Get(
		"/hello/:name",
		fragment.Define("/layout/:name", fragment.WithChild("fragment", fragment.Define("/fragment/:name"))),
	)
	require.NoError(t, err)
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

	server := newServer(t, targetServer.URL)
	err := server.Get(
		"/hello/:name",
		fragment.Define("/definitely_missing_and_not_defined/:name"),
	)
	require.NoError(t, err)
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

			var resultErr *multiplexer.ResultError
			require.ErrorAs(t, results.Error(), &resultErr)
			require.Equal(
				t,
				fmt.Sprintf("%s/definitely_missing_and_not_defined/world", targetServer.URL),
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
	viewProxyServer, err := NewServer(targetServer.URL)
	require.NoError(t, err)
	viewProxyServer.Logger = log.New(ioutil.Discard, "", log.Ldate|log.Ltime)
	tripper := &contextTestTripper{}
	viewProxyServer.MultiplexerTripper = tripper

	root := fragment.Define(
		"/layouts/test_layout", fragment.WithoutValidation(),
		fragment.WithChild("header", fragment.Define("/header/:name")),
		fragment.WithChild("body", fragment.Define("/body/:name")),
		fragment.WithChild("footer", fragment.Define("/footer/:name")),
	)

	err = viewProxyServer.Get("/hello/:name", root)
	require.NoError(t, err)

	r := httptest.NewRequest("GET", "/hello/world?important=true&name=override", nil)
	w := httptest.NewRecorder()

	viewProxyServer.CreateHandler().ServeHTTP(w, r)

	resp := w.Result()

	require.Equal(t, 200, resp.StatusCode)
	require.Equal(t, 4, len(tripper.requestables))
	require.NotNil(t, tripper.route)
}

func TestIgnoreTrailingSlash(t *testing.T) {
	viewProxyServer, err := NewServer(targetServer.URL)
	require.NoError(t, err)
	viewProxyServer.Logger = log.New(ioutil.Discard, "", log.Ldate|log.Ltime)
	viewProxyServer.IgnoreTrailingSlash = true

	root := fragment.Define(
		"/layouts/test_layout", fragment.WithoutValidation(),
		fragment.WithChild("header", fragment.Define("/header/:name")),
		fragment.WithChild("body", fragment.Define("/body/:name")),
		fragment.WithChild("footer", fragment.Define("/footer/:name")),
	)

	err = viewProxyServer.Get("/hello/:name", root)
	require.NoError(t, err)

	r := httptest.NewRequest("GET", "/hello/world/?important=true&name=override", nil)
	w := httptest.NewRecorder()

	viewProxyServer.CreateHandler().ServeHTTP(w, r)
	resp := w.Result()
	require.Equal(t, 200, resp.StatusCode)

	r = httptest.NewRequest("GET", "/hello/world/?important=true&name=override", nil)
	w = httptest.NewRecorder()

	viewProxyServer.IgnoreTrailingSlash = false
	viewProxyServer.CreateHandler().ServeHTTP(w, r)
	resp = w.Result()
	require.Equal(t, 404, resp.StatusCode)
}

func TestWithPassThrough_Error(t *testing.T) {
	_, err := NewServer(targetServer.URL, WithPassThrough("%invalid%"))

	require.Error(t, err)
	require.Contains(t, err.Error(), "viewproxy.ServerOption error")
	require.Contains(t, err.Error(), "WithPassThrough error")
}

func BenchmarkServer(b *testing.B) {
	viewProxyServer := newServer(b, targetServer.URL)
	viewProxyServer.Addr = "localhost:9997"
	viewProxyServer.Logger = log.New(ioutil.Discard, "", log.Ldate|log.Ltime)

	viewProxyServer.AroundResponse = func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			rw.Header().Del("etag")
			next.ServeHTTP(rw, r)
		})
	}

	root := fragment.Define(
		"/layouts/test_layout", fragment.WithoutValidation(), fragment.WithChildren(fragment.Children{
			"header": fragment.Define("/header/:name"),
			"body":   fragment.Define("/body/:name"),
			"name":   fragment.Define("/footer/:name"),
		}),
	)
	viewProxyServer.Get("/hello/:name", root)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		r := httptest.NewRequest("GET", "/hello/world", nil)
		w := httptest.NewRecorder()

		viewProxyServer.CreateHandler().ServeHTTP(w, r)
	}
}

func startTargetServer() *httptest.Server {
	instance := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(r.URL.EscapedPath(), "/")
		name, err := url.PathUnescape(parts[len(parts)-1])

		w.Header().Set("EtAg", "1234")
		w.Header().Set("X-Name", "viewproxy")

		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
		}

		if r.URL.Path == "/layouts/test_layout" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`<html><viewproxy-fragment id="header"></viewproxy-fragment><viewproxy-fragment id="body"></viewproxy-fragment><viewproxy-fragment id="footer"></viewproxy-fragment></html>`))
		} else if strings.HasPrefix(r.URL.Path, "/header/") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("<body>"))
		} else if strings.HasPrefix(r.URL.Path, "/body/") {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(fmt.Sprintf("hello %s", name)))
			if r.URL.Query().Get("important") != "" {
				w.Write([]byte("!"))
			}
		} else if strings.HasPrefix(r.URL.Path, "/footer/") {
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

func newServer(tb testing.TB, target string, opts ...ServerOption) *Server {
	server, err := NewServer(target, opts...)
	require.NoError(tb, err)

	return server
}

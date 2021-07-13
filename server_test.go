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
	"testing"
	"time"

	"github.com/blakewilliams/viewproxy/pkg/multiplexer"
	"github.com/stretchr/testify/assert"
)

var targetServer *httptest.Server

func TestMain(m *testing.M) {
	targetServer = startTargetServer()
	defer targetServer.CloseClientConnections()
	defer targetServer.Close()

	os.Exit(m.Run())
}

func TestServer(t *testing.T) {
	viewProxyServer := NewServer(targetServer.URL)
	viewProxyServer.Port = 9998
	viewProxyServer.Logger = log.New(ioutil.Discard, "", log.Ldate|log.Ltime)

	viewProxyServer.IgnoreHeader("etag")
	layout := NewFragment("/layouts/test_layout")
	fragments := []*Fragment{
		NewFragment("header"),
		NewFragment("body"),
		NewFragment("footer"),
	}
	viewProxyServer.Get("/hello/:name", layout, fragments)

	// Load routes from config
	file, err := ioutil.TempFile(os.TempDir(), "config.json")
	if err != nil {
		t.Error(err)
	}
	defer os.Remove(file.Name())

	file.Write([]byte(`[{
		"url": "/greetings/:name",
		"layout": { "path": "/layouts/test_layout", "metadata": { "foo": "test_layout" }},
		"fragments": [
			{ "path": "header", "metadata": { "foo": "header" }},
			{ "path": "body",   "metadata": { "foo": "body" }},
			{ "path": "footer", "metadata": { "foo": "footer" }}
		]
	}]`))

	file.Close()

	viewProxyServer.Logger = log.New(os.Stdout, "", log.Ldate|log.Ltime)

	err = viewProxyServer.LoadRoutesFromFile(file.Name())
	assert.Nil(t, err)

	go func() {
		if err := viewProxyServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()
	defer viewProxyServer.Close()

	tests := map[string]struct {
		url           string
		expected_body string
	}{
		"basic server": {
			url:           "/hello/world",
			expected_body: "<html><body>hello world</body></html>",
		},

		"json configured server": {
			url:           "/greetings/world",
			expected_body: "<html><body>hello world</body></html>",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {

			resp, err := http.Get(fmt.Sprintf("http://localhost:9998%s", tc.url))
			assert.Nil(t, err)
			body, err := ioutil.ReadAll(resp.Body)
			assert.Nil(t, err)

			assert.Equal(t, tc.expected_body, string(body))
			assert.Equal(t, "viewproxy", resp.Header.Get("x-name"), "Expected response to have an X-Name header")
			assert.Equal(t, "", resp.Header.Get("etag"), "Expected response to have removed etag header")
		})
	}
}

func TestHealthCheck(t *testing.T) {
	viewProxyServer := NewServer(targetServer.URL)
	viewProxyServer.Logger = log.New(ioutil.Discard, "", log.Ldate|log.Ltime)

	r := httptest.NewRequest("GET", "/_ping", nil)
	w := httptest.NewRecorder()

	viewProxyServer.CreateHandler().ServeHTTP(w, r)

	resp := w.Result()

	body, err := ioutil.ReadAll(resp.Body)
	assert.Nil(t, err)
	expected := "200 ok"

	assert.Equal(t, expected, string(body))
}

func TestQueryParamForwardingServer(t *testing.T) {
	viewProxyServer := NewServer(targetServer.URL)
	viewProxyServer.Logger = log.New(ioutil.Discard, "", log.Ldate|log.Ltime)

	viewProxyServer.IgnoreHeader("etag")
	layout := NewFragment("/layouts/test_layout")
	fragments := []*Fragment{
		NewFragment("header"),
		NewFragment("body"),
		NewFragment("footer"),
	}
	viewProxyServer.Get("/hello/:name", layout, fragments)

	r := httptest.NewRequest("GET", "/hello/world?important=true&name=override", nil)
	w := httptest.NewRecorder()

	viewProxyServer.CreateHandler().ServeHTTP(w, r)

	resp := w.Result()

	body, err := ioutil.ReadAll(resp.Body)
	assert.Nil(t, err)
	expected := "<html><body>hello world!</body></html>"

	assert.Equal(t, expected, string(body))
	assert.Equal(t, "viewproxy", resp.Header.Get("x-name"), "Expected response to have an X-Name header")
	assert.Equal(t, "", resp.Header.Get("etag"), "Expected response to have removed etag header")
}

func TestPassThroughEnabled(t *testing.T) {
	viewProxyServer := NewServer(targetServer.URL)
	viewProxyServer.Logger = log.New(ioutil.Discard, "", log.Ldate|log.Ltime)
	viewProxyServer.PassThrough = true

	r := httptest.NewRequest("GET", "/oops", nil)
	w := httptest.NewRecorder()

	viewProxyServer.CreateHandler().ServeHTTP(w, r)

	resp := w.Result()
	body, err := ioutil.ReadAll(resp.Body)
	assert.Nil(t, err)

	assert.Equal(t, 500, resp.StatusCode)
	assert.Equal(t, "Something went wrong", string(body))
}

func TestPassThroughDisabled(t *testing.T) {
	viewProxyServer := NewServer(targetServer.URL)
	viewProxyServer.PassThrough = false

	r := httptest.NewRequest("GET", "/hello/world", nil)
	w := httptest.NewRecorder()

	viewProxyServer.CreateHandler().ServeHTTP(w, r)

	resp := w.Result()
	body, err := ioutil.ReadAll(resp.Body)
	assert.Nil(t, err)

	assert.Equal(t, 404, resp.StatusCode)
	assert.Equal(t, "404 not found", string(body))
}

func TestPassThroughSetsCorrectHeaders(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	done := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(done)

		assert.Equal(t, "", r.Header.Get("Keep-Alive"), "Expected Keep-Alive to be filtered")
		assert.NotEqual(t, "", r.Header.Get("X-Forwarded-For"))
		assert.Equal(t, "localhost:1", r.Header.Get("X-Forwarded-Host"))

		w.Header().Set("Server-Timing", "db;dur=53")
		w.WriteHeader(http.StatusOK)
	}))

	viewProxyServer := NewServer(server.URL)
	viewProxyServer.PassThrough = true

	r := httptest.NewRequest("GET", "/hello/world", nil)
	r.Host = "localhost:1" // go deletes the Host header and sets the Host field
	r.RemoteAddr = "localhost:1"
	w := httptest.NewRecorder()

	viewProxyServer.CreateHandler().ServeHTTP(w, r)

	select {
	case <-done:
		server.Close()
	case <-ctx.Done():
		assert.Fail(t, ctx.Err().Error())
	}

	resp := w.Result()

	assert.Equal(t, "db;dur=53", resp.Header.Get("Server-Timing"))
}

func TestPassThroughPostRequest(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	done := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(done)

		body, err := io.ReadAll(r.Body)

		assert.Nil(t, err)
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "hello", string(body))
	}))

	viewProxyServer := NewServer(server.URL)
	viewProxyServer.PassThrough = true

	r := httptest.NewRequest("POST", "/hello/world", strings.NewReader("hello"))
	w := httptest.NewRecorder()

	viewProxyServer.CreateHandler().ServeHTTP(w, r)

	select {
	case <-done:
		server.Close()
	case <-ctx.Done():
		assert.Fail(t, ctx.Err().Error())
	}
}

func TestFragmentSendsVerifiableHmacWhenSet(t *testing.T) {
	done := make(chan struct{})
	secret := "6ccd9547b7042e0f1101ce68931d6b2c"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(done)

		time := r.Header.Get("X-Authorization-Time")
		assert.NotEqual(t, "", time, "Expected X-Authorization-Time header to be present")

		key := fmt.Sprintf("%s?%s,%s", r.URL.Path, r.URL.RawQuery, time)

		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(
			[]byte(key),
		)

		authorization := r.Header.Get("Authorization")
		assert.NotEqual(t, "", authorization, "Expected Authorization header to be present")

		expected := hex.EncodeToString(mac.Sum(nil))

		assert.Equal(t, expected, authorization)

		w.WriteHeader(http.StatusOK)
	}))

	viewProxyServer := NewServer(server.URL)
	viewProxyServer.Get("/hello/:name", NewFragment("/foo"), []*Fragment{})
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
		assert.Equal(t, "", r.Header.Get("Keep-Alive"), "Expected Keep-Alive to be filtered")
		assert.NotEqual(t, "", r.Header.Get("X-Forwarded-For"))
		assert.Equal(t, "localhost:1", r.Header.Get("X-Forwarded-Host"))
		w.WriteHeader(http.StatusOK)
	}))

	viewProxyServer := NewServer(server.URL)
	layout := NewFragment("/foo")
	layout.TimingLabel = "foo"
	fragment := NewFragment("/bar")
	fragment.TimingLabel = "bar"
	viewProxyServer.Get("/hello/:name", layout, []*Fragment{fragment})

	r := httptest.NewRequest("GET", "/hello/world", strings.NewReader("hello"))
	r.Host = "localhost:1" // go deletes the Host header and sets the Host field
	r.RemoteAddr = "localhost:1"
	w := httptest.NewRecorder()

	viewProxyServer.CreateHandler().ServeHTTP(w, r)

	<-layoutDone
	<-fragmentDone

	resp := w.Result()

	assert.Contains(t, resp.Header.Get("Server-Timing"), "foo-db;desc=\"foo db\";dur=12")
	assert.Contains(t, resp.Header.Get("Server-Timing"), "bar-db;desc=\"bar db\";dur=34")
	assert.Contains(t, resp.Header.Get("Server-Timing"), "foo-fragment;desc=\"foo fragment\";dur=")
	assert.Contains(t, resp.Header.Get("Server-Timing"), "bar-fragment;desc=\"bar fragment\";dur=")

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

	viewProxyServer := NewServer(server.URL)
	viewProxyServer.Get("/hello/:name", NewFragment("/layout"), []*Fragment{NewFragment("/fragment")})

	r := httptest.NewRequest("GET", "/hello/world", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	viewProxyServer.CreateHandler().ServeHTTP(w, r)

	resp := w.Result()

	gzReader, err := gzip.NewReader(resp.Body)
	assert.Nil(t, err)

	body, err := ioutil.ReadAll(gzReader)
	assert.Nil(t, err)

	assert.Equal(t, "<body>wow gzipped!</body>", string(body))

	server.Close()
}

func TestAroundRequestCallback(t *testing.T) {
	done := make(chan struct{})

	server := NewServer("http://fake.net")
	server.Get("/hello/:name", NewFragment("/layout"), []*Fragment{NewFragment("/fragment")})
	server.AroundRequest = func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer close(done)
			w.Header().Set("x-viewproxy", "true")
			assert.Equal(t, "/hello/:name", RouteFromContext(r.Context()).Path)
			assert.Equal(t, "192.168.1.1", r.RemoteAddr)
			next.ServeHTTP(w, r)
		})
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/hello/world", nil)
	r.RemoteAddr = "192.168.1.1"

	server.CreateHandler().ServeHTTP(w, r)

	resp := w.Result()

	assert.Equal(t, "true", resp.Header.Get("x-viewproxy"))

	<-done
}

func TestOnErrorHandler(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	done := make(chan struct{})

	server := NewServer(targetServer.URL)
	server.Get("/hello/:name", NewFragment("/definitely_missing_and_not_defined"), []*Fragment{})
	server.AroundRequest = func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("x-viewproxy", "true")
			assert.Equal(t, "192.168.1.1", r.RemoteAddr)
			next.ServeHTTP(w, r)
		})
	}
	server.OnError = func(w http.ResponseWriter, r *http.Request, e error) {
		defer close(done)
		var resultErr *ResultError

		assert.ErrorAs(t, e, &resultErr)
		assert.Equal(
			t,
			fmt.Sprintf("%s/definitely_missing_and_not_defined?name=world", targetServer.URL),
			resultErr.Result.Url,
		)
		assert.Equal(t, 404, resultErr.Result.StatusCode)
	}

	fakeWriter := httptest.NewRecorder()
	fakeRequest := httptest.NewRequest("GET", "/hello/world", nil)
	fakeRequest.RemoteAddr = "192.168.1.1"

	server.CreateHandler().ServeHTTP(fakeWriter, fakeRequest)

	assert.Equal(t, "true", fakeWriter.Header().Get("x-viewproxy"))

	select {
	case <-done:
	case <-ctx.Done():
		assert.Fail(t, ctx.Err().Error())
	}
}

type contextTestTripper struct {
	route     *Route
	fragments []*multiplexer.Fragment
}

func (t *contextTestTripper) Request(r *http.Request) (*http.Response, error) {
	t.route = RouteFromContext(r.Context())
	t.fragments = append(t.fragments, FragmentFromContext(r.Context()))
	return http.DefaultClient.Do(r)
}

func TestRoundTripperContext(t *testing.T) {
	viewProxyServer := NewServer(targetServer.URL)
	viewProxyServer.Logger = log.New(ioutil.Discard, "", log.Ldate|log.Ltime)
	tripper := &contextTestTripper{}
	viewProxyServer.MultiplexerTripper = tripper

	viewProxyServer.IgnoreHeader("etag")
	layout := NewFragment("/layouts/test_layout")
	fragments := []*Fragment{
		NewFragment("header"),
		NewFragment("body"),
		NewFragment("footer"),
	}
	viewProxyServer.Get("/hello/:name", layout, fragments)

	r := httptest.NewRequest("GET", "/hello/world?important=true&name=override", nil)
	w := httptest.NewRecorder()

	viewProxyServer.CreateHandler().ServeHTTP(w, r)

	resp := w.Result()

	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, 4, len(tripper.fragments))
	assert.NotNil(t, tripper.route)
}

func startTargetServer() *httptest.Server {
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

package viewproxy

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"testing"
)

func TestBasicServer(t *testing.T) {
	targetServer := startTargetServer()
	defer targetServer.Shutdown(context.TODO())

	viewProxyServer := NewServer("http://localhost:9994")
	viewProxyServer.Port = 9998
	viewProxyServer.Logger = log.New(ioutil.Discard, "", log.Ldate|log.Ltime)

	viewProxyServer.IgnoreHeader("etag")
	viewProxyServer.Get("/hello/:name", "/layouts/test_layout", []string{"header", "body", "footer"})

	go func() {
		if err := viewProxyServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()
	defer viewProxyServer.Close()

	resp, err := http.Get("http://localhost:9998/hello/world")

	if err != nil {
		t.Fatal(err)
	}

	body, err := ioutil.ReadAll(resp.Body)
	expected := "<html><body>hello world</body></html>"

	assert.Equal(t, expected, string(body))
	assert.Equal(t, "viewproxy", resp.Header.Get("x-name"), "Expected response to have an X-Name header")
	assert.Equal(t, "", resp.Header.Get("etag"), "Expexted response to have removed etag header")
}

func TestServerFromConfig(t *testing.T) {
	targetServer := startTargetServer()
	defer targetServer.Shutdown(context.TODO())

	file, err := ioutil.TempFile(os.TempDir(), "config.json")
	if err != nil {
		t.Error(err)
	}
	defer os.Remove(file.Name())

	file.Write([]byte(`[{
		"url": "/hello/:name",
		"layout": "/layouts/test_layout",
		"fragments": ["header", "body", "footer"]
	}]`))

	file.Close()

	viewProxyServer := NewServer("http://localhost:9994")
	viewProxyServer.Port = 9998
	viewProxyServer.Logger = log.New(ioutil.Discard, "", log.Ldate|log.Ltime)

	viewProxyServer.LoadRouteConfig(file.Name())
	go func() {
		if err := viewProxyServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()
	defer viewProxyServer.Close()

	resp, err := http.Get("http://localhost:9998/hello/world")

	if err != nil {
		t.Fatal(err)
	}

	body, err := ioutil.ReadAll(resp.Body)
	expected := "<html><body>hello world</body></html>"

	assert.Equal(t, expected, string(body))
}

func TestPassThroughEnabled(t *testing.T) {
	targetServer := startTargetServer()
	defer targetServer.Shutdown(context.TODO())

	viewProxyServer := NewServer("http://localhost:9994")
	viewProxyServer.Port = 9995
	viewProxyServer.Logger = log.New(ioutil.Discard, "", log.Ldate|log.Ltime)
	viewProxyServer.PassThrough = true

	go func() {
		if err := viewProxyServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()
	defer viewProxyServer.Close()

	resp, err := http.Get("http://localhost:9995/hello/world")

	if err != nil {
		t.Fatal(err)
	}

	body, err := ioutil.ReadAll(resp.Body)

	assert.Equal(t, 404, resp.StatusCode)
	assert.Equal(t, "target: 404 not found", string(body))
}

func TestPassThroughDisabled(t *testing.T) {
	targetServer := startTargetServer()
	defer targetServer.Shutdown(context.TODO())

	viewProxyServer := NewServer("http://localhost:9994")
	viewProxyServer.Port = 9993
	viewProxyServer.Logger = log.New(ioutil.Discard, "", log.Ldate|log.Ltime)
	viewProxyServer.PassThrough = false

	go func() {
		if err := viewProxyServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()
	defer viewProxyServer.Close()

	resp, err := http.Get("http://localhost:9993/hello/world")

	if err != nil {
		t.Fatal(err)
	}

	body, err := ioutil.ReadAll(resp.Body)

	assert.Equal(t, 404, resp.StatusCode)
	assert.Equal(t, "404 not found", string(body))
}

func startTargetServer() *http.Server {
	instance := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		params := r.URL.Query()

		w.Header().Set("EtAg", "1234")
		w.Header().Set("X-Name", "viewproxy")

		if r.URL.Path == "/layouts/test_layout" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("<html>{{{VIEW_PROXY_CONTENT}}}</html>"))
		} else if r.URL.Path == "/header" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("<body>"))
		} else if r.URL.Path == "/body" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(fmt.Sprintf("hello %s", params.Get("name"))))
		} else if r.URL.Path == "/footer" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("</body>"))
		} else {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("target: 404 not found"))
		}
	})

	testServer := &http.Server{Addr: ":9994", Handler: instance}
	go func() {
		if err := testServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()

	return testServer
}

package viewproxy

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"log"
	"net/http"
	"testing"
	"time"
)

func TestBasicServer(t *testing.T) {
	instance := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		params := r.URL.Query()

		w.Header().Set("EtAg", "1234")
		w.Header().Set("X-Name", "viewproxy")

		w.WriteHeader(http.StatusOK)

		if r.URL.Path == "/layouts/test_layout" {
			w.Write([]byte("<html>{{{VIEW_PROXY_CONTENT}}}</html>"))
		} else if r.URL.Path == "/header" {
			w.Write([]byte("<body>"))
		} else if r.URL.Path == "/body" {
			w.Write([]byte(fmt.Sprintf("hello %s", params.Get("name"))))
		} else if r.URL.Path == "/footer" {
			w.Write([]byte("</body>"))
		}
	})

	testServer := &http.Server{Addr: ":9994", Handler: instance}
	go func() {
		if err := testServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()

	viewProxyServer := &Server{
		Port:         9998,
		Target:       "http://localhost:9994",
		Logger:       log.New(ioutil.Discard, "", log.Ldate|log.Ltime),
		ProxyTimeout: time.Duration(5) * time.Second,
	}

	viewProxyServer.IgnoreHeader("etag")
	viewProxyServer.Get("/hello/:name", "test_layout", []string{"header", "body", "footer"})

	go func() {
		if err := viewProxyServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()

	resp, err := http.Get("http://localhost:9998/hello/world")

	if err != nil {
		t.Fatal(err)
	}

	body, err := ioutil.ReadAll(resp.Body)
	expected := "<html><body>hello world</body></html>"

	assert.Equal(t, expected, string(body))

	assert.Equal(t, "viewproxy", resp.Header.Get("x-name"), "Expected response to have an X-Name header")
	assert.Equal(t, "", resp.Header.Get("etag"), "Expexted response to have removed etag header")

	testServer.Shutdown(context.TODO())
	viewProxyServer.Shutdown(context.TODO())
}

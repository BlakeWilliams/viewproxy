package server

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"testing"
)

func TestBasicServer(t *testing.T) {
	instance := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		params := r.URL.Query()
		fragment := params.Get("fragment")

		w.WriteHeader(http.StatusOK)

		if fragment == "header" {
			w.Write([]byte("<body>"))
		} else if fragment == "body" {
			w.Write([]byte(fmt.Sprintf("hello %s", params.Get("name"))))
		} else if fragment == "footer" {
			w.Write([]byte("</body>"))
		}
	})

	testServer := &http.Server{Addr: ":9999", Handler: instance}
	go func() {
		if err := testServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()

	viewProxyServer := &Server{
		Port:   9998,
		Target: "http://localhost:9999",
		Logger: log.New(ioutil.Discard, "", log.Ldate|log.Ltime),
	}

	viewProxyServer.Get("/hello/:name", []string{"header", "body", "footer"})
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
	expected := "<body>hello world</body>"

	if string(body) != expected {
		t.Fatalf("Expected: %s\nGot: %s", expected, string(body))
	}

	testServer.Shutdown(context.TODO())
	viewProxyServer.Shutdown(context.TODO())
}

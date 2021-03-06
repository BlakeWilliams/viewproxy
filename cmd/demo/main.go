package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/blakewilliams/viewproxy"
	"github.com/blakewilliams/viewproxy/pkg/fragment"
	"github.com/blakewilliams/viewproxy/pkg/middleware/logging"
	"github.com/blakewilliams/viewproxy/pkg/multiplexer"
)

func main() {
	target := getTarget()
	server, err := viewproxy.NewServer(target, viewproxy.WithPassThrough(target))

	if err != nil {
		panic(err)
	}

	server.Addr = fmt.Sprintf("localhost:%d", getPort())
	server.ProxyTimeout = time.Duration(5) * time.Second
	server.Logger = buildLogger()

	server.Get(
		"/hello/:name",
		fragment.Define("/layout/:name", fragment.WithChildren(fragment.Children{
			"body": fragment.Define("/body/:name", fragment.WithChildren(fragment.Children{
				"header":  fragment.Define("/header/:name", fragment.WithMetadata(map[string]string{"title": "Hello"})),
				"message": fragment.Define("/message/:name"),
			})),
		})),
	)

	server.AroundResponse = func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			// Strip etag header from response
			rw.Header().Del("etag")
			h.ServeHTTP(rw, r)
		})
	}

	// setup middleware
	server.AroundRequest = func(handler http.Handler) http.Handler {
		handler = logging.Middleware(server, server.Logger)(handler)

		return handler
	}

	server.MultiplexerTripper = logging.NewLogTripper(
		server.Logger,
		server.SecretFilter,
		multiplexer.NewStandardTripper(&http.Client{}),
	)

	server.ListenAndServe()
}

func buildLogger() *log.Logger {
	file, err := os.OpenFile("log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	defer file.Close()

	if err != nil {
		log.Fatal(err)
	}
	return log.New(io.MultiWriter(os.Stdout, file), "", log.Ldate|log.Ltime)
}

func getPort() int {
	if _, ok := os.LookupEnv("PORT"); ok {
		port, err := strconv.Atoi(os.Getenv("PORT"))

		if err != nil {
			panic(err)
		}

		return port
	}

	return 3005
}

func getTarget() string {
	if value, ok := os.LookupEnv("TARGET"); ok {
		return value
	}

	return "http://localhost:3000/"
}

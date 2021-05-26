package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Server struct {
	port         int
	proxyTimeout time.Duration
	routes       []Route
	target       string
}

func (s *Server) Get(path string, fragments []string) {
	route := newRoute(path, fragments)
	s.routes = append(s.routes, *route)
}

// TODO this should probably be a tree structure for faster lookups
func (s *Server) MatchingRoute(path string) (*Route, map[string]string) {
	parts := strings.Split(path, "/")

	for _, route := range s.routes {
		if route.MatchParts(parts) {
			parameters := route.ParametersFor(parts)
			return &route, parameters
		}
	}

	return nil, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	route, parameters := s.MatchingRoute(r.URL.Path)
	paramString := ""

	// TODO use a URL struct for url generation
	for name, value := range parameters {
		// TODO use url.QueryEscape
		paramString = paramString + fmt.Sprintf("&%s=%s", name, url.QueryEscape(value))
	}

	if route != nil {
		fragmentContent := make([]chan []byte, 0)

		for _, fragment := range route.fragments {
			content := make(chan []byte)
			fragmentContent = append(fragmentContent, content)

			go func(fragment string) {
				// TODO handle errors
				url := fmt.Sprintf("%s?fragment=%s%s", s.target, fragment, paramString)
				fmt.Printf("Fetching: %s\n", url)
				resp, _ := http.Get(url)
				// TODO handle errors
				body, _ := ioutil.ReadAll(resp.Body)
				content <- body
			}(fragment)
		}

		for _, content := range fragmentContent {
			body := <-content

			w.Write(body)
		}
	} else {
		w.Write([]byte("404 not found"))
	}
}

func (s *Server) ListenAndServe() {
	httpServer := &http.Server{
		Addr:           fmt.Sprintf(":%d", s.port),
		Handler:        s,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	log.Fatal(httpServer.ListenAndServe())
}

func main() {
	timeout, err := time.ParseDuration("5s")

	if err != nil {
		panic(err)
	}

	port := 3005

	if _, ok := os.LookupEnv("PORT"); ok {
		port, err = strconv.Atoi(os.Getenv("PORT"))

		if err != nil {
			panic(err)
		}
	}

	target := "http://localhost:3000/_view_fragments"
	if value, ok := os.LookupEnv("TARGET"); ok {
		target = value
	}

	server := &Server{
		port:         port,
		proxyTimeout: timeout,
		target:       target,
	}

	server.Get("/hello/:name", []string{
		"header",
		"hello",
		"footer",
	})

	server.ListenAndServe()
}

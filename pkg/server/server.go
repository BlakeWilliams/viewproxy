package server

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Server struct {
	Port         int
	ProxyTimeout time.Duration
	routes       []Route
	Target       string
	Logger       *log.Logger
	httpServer   *http.Server
}

func (s *Server) Get(path string, fragments []string) {
	route := newRoute(path, fragments)
	s.routes = append(s.routes, *route)
}

func (s *Server) Shutdown(ctx context.Context) {
	s.httpServer.Shutdown(ctx)
}

// TODO this should probably be a tree structure for faster lookups
func (s *Server) matchingRoute(path string) (*Route, map[string]string) {
	parts := strings.Split(path, "/")

	for _, route := range s.routes {
		if route.matchParts(parts) {
			parameters := route.parametersFor(parts)
			return &route, parameters
		}
	}

	return nil, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	route, parameters := s.matchingRoute(r.URL.Path)
	paramString := ""

	// TODO use a URL struct for url generation
	for name, value := range parameters {
		paramString = paramString + fmt.Sprintf("&%s=%s", name, url.QueryEscape(value))
	}

	if route != nil {
		s.Logger.Printf("Handling %s\n", r.URL.Path)

		fragmentContent := make([]chan []byte, 0)

		for _, fragment := range route.fragments {
			content := make(chan []byte)
			fragmentContent = append(fragmentContent, content)

			go func(fragment string) {
				start := time.Now()

				// TODO handle errors
				url := fmt.Sprintf("%s?fragment=%s%s", s.Target, fragment, paramString)
				resp, _ := http.Get(url)
				duration := time.Since(start)

				s.Logger.Printf("Fetched %s?fragment=%s%s in %v", s.Target, fragment, paramString, duration)

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
		s.Logger.Printf("Rendering 404 for %s\n", r.URL.Path)
		w.Write([]byte("404 not found"))
	}
}

func (s *Server) ListenAndServe() error {
	s.httpServer = &http.Server{
		Addr:           fmt.Sprintf(":%d", s.Port),
		Handler:        s,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	s.Logger.Printf("Listening on port %d\n", s.Port)
	return s.httpServer.ListenAndServe()
}

package viewproxy

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/blakewilliams/viewproxy/pkg/multiplexer"
)

type Server struct {
	Port         int
	ProxyTimeout time.Duration
	routes       []Route
	Target       string
	Logger       *log.Logger
	httpServer   *http.Server
}

func (s *Server) Get(path string, layout string, fragments []string) {
	route := newRoute(path, layout, fragments)
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

	if route != nil {
		s.Logger.Printf("Handling %s\n", r.URL.Path)

		urls := make([]string, 0)

		urls = append(urls, s.constructLayoutUrl(route.Layout, parameters))

		for _, fragment := range route.fragments {
			urls = append(urls, s.constructFragmentUrl(fragment, parameters))
		}

		results, err := multiplexer.Fetch(context.TODO(), urls, s.ProxyTimeout)

		if err != nil {
			// TODO detect 404's and 500's and handle them appropriately
			s.Logger.Printf("Errored %v", err)
		}

		layoutHtml := results[0].Body
		contentHtml := []byte("")

		for _, result := range results[1:] {
			s.Logger.Printf("Fetched %s in %v", result.Url, result.Duration)
			contentHtml = append(contentHtml, result.Body...)
		}

		outputHtml := bytes.Replace(layoutHtml, []byte("{{{VOLTRON_CONTENT}}}"), contentHtml, 1)
		w.Write(outputHtml)
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

func (s *Server) constructLayoutUrl(layout string, parameters map[string]string) string {
	targetUrl, err := url.Parse(s.Target)
	if err != nil {
		panic(err)
	}

	targetUrl.Path = targetUrl.Path + "/layouts/" + layout

	query := url.Values{}

	for name, value := range parameters {
		query.Add(name, value)
	}

	targetUrl.RawQuery = query.Encode()

	return targetUrl.String()
}

func (s *Server) constructFragmentUrl(fragment string, parameters map[string]string) string {
	targetUrl, err := url.Parse(
		fmt.Sprintf("%s/%s", s.Target, fragment),
	)

	if err != nil {
		panic(err)
	}

	query := url.Values{}

	for name, value := range parameters {
		query.Add(name, value)
	}

	targetUrl.RawQuery = query.Encode()

	return targetUrl.String()
}

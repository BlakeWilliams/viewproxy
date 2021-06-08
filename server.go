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
	Port             int
	ProxyTimeout     time.Duration
	routes           []Route
	Target           string
	Logger           *log.Logger
	httpServer       *http.Server
	DefaultPageTitle string
	ignoreHeaders    map[string]struct{}
	PassThrough      bool
}

var setMember struct{}

func NewServer(target string) *Server {
	return &Server{
		DefaultPageTitle: "viewproxy",
		Logger:           log.Default(),
		Port:             3005,
		ProxyTimeout:     time.Duration(10) * time.Second,
		PassThrough:      false,
		Target:           target,
		ignoreHeaders:    make(map[string]struct{}, 0),
		routes:           make([]Route, 0),
	}
}

func (s *Server) Get(path string, layout string, fragments []string) {
	route := newRoute(path, layout, fragments)
	s.routes = append(s.routes, *route)
}

func (s *Server) IgnoreHeader(name string) {
	s.ignoreHeaders[strings.ToLower(name)] = setMember
}

func (s *Server) LoadRouteConfig(filePath string) error {
	routeEntries, err := readConfigFile(filePath)
	if err != nil {
		return err
	}

	for _, routeEntry := range routeEntries {
		s.Logger.Printf("Defining %s, with layout %s, for fragments %v\n", routeEntry.Url, routeEntry.LayoutFragmentUrl, routeEntry.FragmentUrls)
		s.Get(routeEntry.Url, routeEntry.LayoutFragmentUrl, routeEntry.FragmentUrls)
	}

	return nil
}

func (s *Server) Shutdown(ctx context.Context) {
	s.httpServer.Shutdown(ctx)
}

func (s *Server) Close() {
	s.httpServer.Close()
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

		results, err := multiplexer.Fetch(context.TODO(), urls, http.Header{}, s.ProxyTimeout)

		if err != nil {
			// TODO detect 404's and 500's and handle them appropriately
			s.Logger.Printf("Errored %v", err)
		}

		layoutHtml := results[0].Body
		s.Logger.Printf("Fetched layout %s in %v", results[0].Url, results[0].Duration)

		contentHtml := []byte("")
		pageTitle := s.DefaultPageTitle

		for name, values := range results[0].HttpResponse.Header {
			if _, ok := s.ignoreHeaders[strings.ToLower(name)]; !ok {
				for _, value := range values {
					w.Header().Add(name, value)
				}
			}
		}

		for _, result := range results[1:] {
			s.Logger.Printf("Fetched %s in %v", result.Url, result.Duration)
			contentHtml = append(contentHtml, result.Body...)

			if result.HttpResponse.Header.Get("X-View-Proxy-Title") != "" {
				pageTitle = result.HttpResponse.Header.Get("X-View-Proxy-Title")
			}
		}

		outputHtml := bytes.Replace(layoutHtml, []byte("{{{VIEW_PROXY_CONTENT}}}"), contentHtml, 1)
		outputHtml = bytes.Replace(outputHtml, []byte("{{{VIEW_PROXY_PAGE_TITLE}}}"), []byte(pageTitle), 1)
		w.Write(outputHtml)
	} else if s.PassThrough {
		targetUrl, err := url.Parse(
			fmt.Sprintf("%s/%s", s.Target, r.URL.String()),
		)

		if err != nil {
			s.handleProxyError(err, w)
			return
		}

		result, err := multiplexer.ProxyRequest(context.TODO(), targetUrl.String(), r)

		if err != nil {
			s.handleProxyError(err, w)
			return
		}

		for name, values := range result.HeadersWithoutProxyHeaders() {
			w.Header()[name] = values
		}

		w.WriteHeader(result.StatusCode)
		w.Write(result.Body)

		s.Logger.Printf("Proxied %s in %v", result.Url, result.Duration)
	} else {
		s.Logger.Printf("Rendering 404 for %s\n", r.URL.Path)
		w.WriteHeader(404)
		w.Write([]byte("404 not found"))
	}
}

func (s *Server) handleProxyError(err error, w http.ResponseWriter) {
	s.Logger.Printf("Pass through error: %v", err)
	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte("Internal Server Error"))
	return
}

func (s *Server) ListenAndServe() error {
	s.IgnoreHeader("Content-Length")

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

	targetUrl.Path = targetUrl.Path + layout

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

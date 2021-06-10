package viewproxy

import (
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
	target           string
	Logger           *log.Logger
	httpServer       *http.Server
	DefaultPageTitle string
	ignoreHeaders    []string
	PassThrough      bool
	// Sets the secret used to generate an HMAC that can be used by the target
	// server to validate that a request came from viewproxy.
	//
	// When set, two headers are sent to the target URL for fragment and layout
	// requests. The `X-Authorization-Timestamp` header, which is a timestamp
	// generated at the start of the request, and `X-Authorization`, which is a
	// hex encoded HMAC of "urlWithQueryParams,timestamp`.
	HmacSecret string
}

var setMember struct{}

func NewServer(target string) *Server {
	return &Server{
		DefaultPageTitle: "viewproxy",
		Logger:           log.Default(),
		Port:             3005,
		ProxyTimeout:     time.Duration(10) * time.Second,
		PassThrough:      false,
		target:           target,
		ignoreHeaders:    make([]string, 0),
		routes:           make([]Route, 0),
	}
}

func (s *Server) Get(path string, layout string, fragments []string) {
	baseLayoutUrl := s.urlFromTarget(layout)
	baseFragmentUrls := make([]string, len(fragments))

	for i, fragment := range fragments {
		baseFragmentUrls[i] = s.urlFromTarget(fragment)
	}

	route := newRoute(path, baseLayoutUrl, baseFragmentUrls)
	s.routes = append(s.routes, *route)
}

func (s *Server) IgnoreHeader(name string) {
	s.ignoreHeaders = append(s.ignoreHeaders, name)
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

		results, err := multiplexer.Fetch(
			context.TODO(),
			route.fragmentsWithParameters(parameters),
			multiplexer.HeadersFromRequest(r),
			s.ProxyTimeout,
			s.HmacSecret,
		)

		if err != nil {
			// TODO detect 404's and 500's and handle them appropriately
			s.Logger.Printf("Errored %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("500 internal server error"))
			return
		}

		s.Logger.Printf("Fetched layout %s in %v", results[0].Url, results[0].Duration)
		for _, result := range results[1:] {
			s.Logger.Printf("Fetched %s in %v", result.Url, result.Duration)
		}

		resBuilder := newResponseBuilder(*s, w)
		resBuilder.SetLayout(results[0])
		resBuilder.SetHeaders(results[0].HeadersWithoutProxyHeaders())
		resBuilder.SetFragments(results[1:])
		resBuilder.Write()
	} else if s.PassThrough {
		targetUrl, err := url.Parse(
			fmt.Sprintf("%s/%s", strings.TrimRight(s.target, "/"), strings.TrimLeft(r.URL.String(), "/")),
		)

		targetUrl.RawQuery = r.URL.Query().Encode()

		if err != nil {
			s.handleProxyError(err, w)
			return
		}

		result, err := multiplexer.FetchUrlWithoutStatusCodeCheck(
			context.TODO(),
			r.Method,
			targetUrl.String(),
			multiplexer.HeadersFromRequest(r),
			r.Body,
		)

		if err != nil {
			s.handleProxyError(err, w)
			return
		}
		s.Logger.Printf("Proxied %s in %v", result.Url, result.Duration)

		resBuilder := newResponseBuilder(*s, w)
		resBuilder.StatusCode = result.StatusCode
		resBuilder.SetHeaders(result.HeadersWithoutProxyHeaders())
		resBuilder.SetFragments([]*multiplexer.Result{result})
		resBuilder.Write()
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

func (s *Server) urlFromTarget(fragment string) string {
	targetUrl, err := url.Parse(
		fmt.Sprintf("%s/%s", strings.TrimRight(s.target, "/"), strings.TrimLeft(fragment, "/")),
	)

	if err != nil {
		// It should be okay to panic here, since this should only be called at boot time
		panic(err)
	}

	return targetUrl.String()
}

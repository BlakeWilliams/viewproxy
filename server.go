package viewproxy

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/blakewilliams/viewproxy/internal/tracing"
	"github.com/blakewilliams/viewproxy/pkg/multiplexer"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// Re-export ResultError for convenience
type ResultError = multiplexer.ResultError

type logger interface {
	Fatal(v ...interface{})
	Fatalf(format string, v ...interface{})
	Fatalln(v ...interface{})
	Panic(v ...interface{})
	Panicf(format string, v ...interface{})
	Panicln(v ...interface{})
	Print(v ...interface{})
	Printf(format string, v ...interface{})
	Println(v ...interface{})
}

type Server struct {
	Port             int
	ProxyTimeout     time.Duration
	routes           []Route
	target           string
	Logger           logger
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
	// hex encoded HMAC of "urlPathWithQueryParams,timestamp`.
	HmacSecret string
	// The transport passed to `http.Client` when fetching fragments or proxying
	// requests.
	HttpTransport http.RoundTripper
	// A function that is called before the request is handled by viewproxy.
	PreRequest    func(w http.ResponseWriter, r *http.Request)
	tracingConfig tracing.TracingConfig
	// A function that is called when an error occurs in the viewproxy handler
	OnError func(w http.ResponseWriter, r *http.Request, e error)
}

func NewServer(target string) *Server {
	return &Server{
		DefaultPageTitle: "viewproxy",
		HttpTransport:    http.DefaultTransport,
		Logger:           log.Default(),
		Port:             3005,
		ProxyTimeout:     time.Duration(10) * time.Second,
		PassThrough:      false,
		PreRequest:       func(http.ResponseWriter, *http.Request) {},
		target:           target,
		ignoreHeaders:    make([]string, 0),
		routes:           make([]Route, 0),
		tracingConfig:    tracing.TracingConfig{Enabled: false},
	}
}

func (s *Server) Get(path string, layout *Fragment, fragments []*Fragment) {
	route := newRoute(path, layout, fragments)

	layout.PreloadUrl(s.target)
	for _, fragment := range fragments {
		fragment.PreloadUrl(s.target)
	}

	s.routes = append(s.routes, *route)
}

func (s *Server) IgnoreHeader(name string) {
	s.ignoreHeaders = append(s.ignoreHeaders, name)
}

func (s *Server) LoadRoutesFromFile(filePath string) error {
	routeEntries, err := readConfigFile(filePath)
	if err != nil {
		return err
	}

	return s.loadRoutes(routeEntries)
}

func (s *Server) LoadRoutesFromJSON(routesJson string) error {
	routeEntries, err := loadJsonConfig([]byte(routesJson))
	if err != nil {
		return err
	}

	return s.loadRoutes(routeEntries)
}

func (s *Server) ConfigureTracing(endpoint string, serviceName string, insecure bool) {
	s.tracingConfig.Enabled = true
	s.tracingConfig.Endpoint = endpoint
	s.tracingConfig.ServiceName = serviceName
	s.tracingConfig.Insecure = insecure
}

func (s *Server) loadRoutes(routeEntries []configRouteEntry) error {
	for _, routeEntry := range routeEntries {
		s.Logger.Printf("Defining %s, with layout %s, for fragments %v\n", routeEntry.Url, routeEntry.Layout, routeEntry.Fragments)
		s.Get(routeEntry.Url, routeEntry.Layout, routeEntry.Fragments)
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
	ctx := context.Background()

	tracer := otel.Tracer("server")
	var span trace.Span
	ctx, span = tracer.Start(ctx, "ServeHTTP")
	defer span.End()

	s.PreRequest(w, r)
	route, parameters := s.matchingRoute(r.URL.Path)

	if route != nil {
		s.Logger.Printf("Handling %s\n", r.URL.Path)
		req := multiplexer.NewRequest()
		req.Timeout = s.ProxyTimeout
		req.Transport = s.HttpTransport
		req.HmacSecret = s.HmacSecret

		for _, f := range route.FragmentsToRequest() {
			query := url.Values{}
			for name, value := range parameters {
				query.Add(name, value)
			}
			req.WithFragment(f.UrlWithParams(query), f.Metadata)
		}

		req.WithHeadersFromRequest(r)
		results, err := req.Do(ctx)

		if err != nil {
			if s.OnError != nil {
				s.OnError(w, r, err)
				return
			} else {
				s.Logger.Printf("Errored %v", err)
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("500 internal server error"))
				return
			}
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

		req := multiplexer.NewRequest()
		req.Timeout = s.ProxyTimeout
		req.Transport = s.HttpTransport
		req.Non2xxErrors = false

		req.WithHeadersFromRequest(r)
		result, err := req.DoSingle(
			ctx,
			r.Method,
			targetUrl.String(),
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
}

func (s *Server) ListenAndServe() error {
	shutdownTracing, err := tracing.Instrument(s.tracingConfig, s.Logger)
	if err != nil {
		log.Printf("Error instrumenting tracing: %v", err)
	}

	defer shutdownTracing()

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

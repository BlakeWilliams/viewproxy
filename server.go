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
	// A function to wrap request handling with other middleware
	AroundRequest func(http.Handler) http.Handler
	tracingConfig tracing.TracingConfig
	// A function that is called when an error occurs in the viewproxy handler
	OnError func(w http.ResponseWriter, r *http.Request, e error)
}

type routeContextKey struct{}

type parametersContextKey struct{}

func NewServer(target string) *Server {
	server := &Server{
		DefaultPageTitle: "viewproxy",
		HttpTransport:    http.DefaultTransport,
		Logger:           log.Default(),
		Port:             3005,
		ProxyTimeout:     time.Duration(10) * time.Second,
		PassThrough:      false,
		AroundRequest:    func(h http.Handler) http.Handler { return h },
		target:           target,
		ignoreHeaders:    make([]string, 0),
		routes:           make([]Route, 0),
		tracingConfig:    tracing.TracingConfig{Enabled: false},
	}

	return server
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

func (s *Server) rootHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		tracer := otel.Tracer("server")
		var span trace.Span
		ctx, span = tracer.Start(ctx, "ServeHTTP")
		defer span.End()

		route, parameters := s.matchingRoute(r.URL.Path)

		if r.URL.Path == "/_ping" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("200 ok"))
			return
		}

		if route != nil {
			ctx = context.WithValue(ctx, routeContextKey{}, route)
			ctx = context.WithValue(ctx, parametersContextKey{}, parameters)
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) requestHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		route := s.GetRoute(ctx)
		if route != nil {
			parameters := s.GetParameters(ctx)
			s.handleRequest(w, r, route, parameters, ctx)
		} else {
			s.passThrough(w, r)
		}
	})
}

func (s *Server) createHandler() http.Handler {
	return s.rootHandler(s.AroundRequest(s.requestHandler()))
}

func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request, route *Route, parameters map[string]string, ctx context.Context) {
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
		for name, values := range r.URL.Query() {
			if query.Get(name) == "" {
				for _, value := range values {
					query.Add(name, value)
				}
			}
		}

		req.WithFragment(f.UrlWithParams(query), f.Metadata, f.TimingLabel)
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
	resBuilder.SetHeaders(results[0].HeadersWithoutProxyHeaders(), results)
	resBuilder.SetFragments(results[1:])
	resBuilder.Write()
}

func (s *Server) passThrough(w http.ResponseWriter, r *http.Request) {
	if s.PassThrough {
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
			r.Context(),
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
		results := []*multiplexer.Result{result}
		resBuilder.SetHeaders(result.HeadersWithoutProxyHeaders(), results)
		resBuilder.SetFragments(results)
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

func (s *Server) GetRoute(ctx context.Context) *Route {
	if ctx == nil {
		return nil
	}

	if route := ctx.Value(routeContextKey{}); route != nil {
		return route.(*Route)
	}
	return nil
}

func (s *Server) GetParameters(ctx context.Context) map[string]string {
	if ctx == nil {
		return nil
	}

	if parameters := ctx.Value(parametersContextKey{}); parameters != nil {
		return parameters.(map[string]string)
	}
	return nil
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
		Handler:        s.createHandler(),
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}

	s.Logger.Printf("Listening on port %d\n", s.Port)

	return s.httpServer.ListenAndServe()
}

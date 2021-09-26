package viewproxy

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/blakewilliams/viewproxy/internal/tracing"
	"github.com/blakewilliams/viewproxy/pkg/fragment"
	"github.com/blakewilliams/viewproxy/pkg/multiplexer"
	"github.com/blakewilliams/viewproxy/pkg/notifier"
	"github.com/blakewilliams/viewproxy/pkg/secretfilter"
)

const (
	HeaderViewProxyOriginalPath = "X-Viewproxy-Original-Path"
)

const (
	EventServeHTTP = "serveHTTP"
	EventProxy     = "proxy"
)

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
	Addr string
	// Sets the maximum duration for requests made to the target server
	ProxyTimeout time.Duration
	// Sets the maximum duration for reading the entire request, including the body
	ReadTimeout time.Duration
	// Sets the maximum duration before timing out writes of the response
	WriteTimeout time.Duration
	// Ignores incoming request's trailing slashes when trying to match a
	// request URL to a route. This only applies to routes that are not declared
	// with an explicit trailing slash.
	IgnoreTrailingSlash bool
	routes              []Route
	target              string
	targetURL           *url.URL
	httpServer          *http.Server
	reverseProxy        *httputil.ReverseProxy
	Logger              logger
	passThrough         bool
	SecretFilter        secretfilter.Filter
	// Sets the secret used to generate an HMAC that can be used by the target
	// server to validate that a request came from viewproxy.
	//
	// When set, two headers are sent to the target URL for fragment and layout
	// requests. The `X-Authorization-Timestamp` header, which is a timestamp
	// generated at the start of the request, and `X-Authorization`, which is a
	// hex encoded HMAC of "urlPathWithQueryParams,timestamp`.
	HmacSecret string
	// The multiplexer.Tripper passed to the multiplexer package
	MultiplexerTripper multiplexer.Tripper
	// A function to wrap the entire request handling with other middleware
	AroundRequest func(http.Handler) http.Handler
	// A function to wrap around the generating of the response after the fragment
	// requests have completed or errored
	AroundResponse func(http.Handler) http.Handler

	// Used to expose hooks in the framework for logging and observability.
	Notifier notifier.Notifier
}

type ServerOption = func(*Server) error

type routeContextKey struct{}
type parametersContextKey struct{}
type startTimeKey struct{}

const defaultTimeout = 10 * time.Second

func emptyMiddleware(h http.Handler) http.Handler { return h }

// NewServer returns a new Server that will make requests to the given target argument.
func NewServer(target string, opts ...ServerOption) (*Server, error) {
	targetURL, err := url.Parse(target)

	if err != nil {
		return nil, err
	}

	server := &Server{
		MultiplexerTripper:  multiplexer.NewStandardTripper(&http.Client{}),
		Logger:              log.Default(),
		SecretFilter:        secretfilter.New(),
		Addr:                "localhost:3005",
		ProxyTimeout:        defaultTimeout,
		ReadTimeout:         defaultTimeout,
		WriteTimeout:        defaultTimeout,
		passThrough:         false,
		AroundRequest:       emptyMiddleware,
		AroundResponse:      emptyMiddleware,
		IgnoreTrailingSlash: true,
		target:              target,
		targetURL:           targetURL,
		routes:              make([]Route, 0),
		tracingConfig:       tracing.TracingConfig{Enabled: false},
		Notifier:            notifier.New(),
	}

	for _, fn := range opts {
		err := fn(server)

		if err != nil {
			return nil, fmt.Errorf("viewproxy.ServerOption error: %w", err)
		}
	}

	return server, nil
}

func WithPassThrough(passthroughTarget string) ServerOption {
	return func(server *Server) error {
		targetURL, err := url.Parse(passthroughTarget)

		if err != nil {
			return fmt.Errorf("WithPassThrough error: %w", err)
		}

		server.passThrough = true
		server.reverseProxy = httputil.NewSingleHostReverseProxy(targetURL)

		return nil
	}
}

func (s *Server) PassThroughEnabled() bool {
	return s.passThrough
}

type GetOption = func(*Route)

func WithRouteMetadata(metadata map[string]string) GetOption {
	return func(route *Route) {
		route.Metadata = metadata
	}
}

func (s *Server) Get(path string, root *fragment.Definition, opts ...GetOption) error {
	route := newRoute(path, map[string]string{}, root)

	for _, opt := range opts {
		opt(route)
	}

	err := route.Validate()
	if err != nil {
		return err
	}

	s.routes = append(s.routes, *route)

	return nil
}

// target returns the configured http target
func (s *Server) Target() string {
	return s.target
}

// routes returns a slice containing routes defined on the server.
func (s *Server) Routes() []Route {
	return s.routes
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) Close() {
	s.httpServer.Close()
}

// TODO this should probably be a tree structure for faster lookups
func (s *Server) MatchingRoute(path string) (*Route, map[string]string) {
	parts := strings.Split(path, "/")

	if s.IgnoreTrailingSlash && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}

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

		route, parameters := s.MatchingRoute(r.URL.EscapedPath())

		if route != nil {
			ctx = context.WithValue(ctx, routeContextKey{}, route)
			ctx = context.WithValue(ctx, parametersContextKey{}, parameters)
		}

		s.Notifier.Emit(EventServeHTTP, ctx, func(ctx context.Context) {
			next.ServeHTTP(w, r.WithContext(ctx))
		})

	})
}

func (s *Server) requestHandler() http.Handler {
	responseHandler := s.createResponseHandler()

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		route := RouteFromContext(ctx)
		if route != nil {
			parameters := ParametersFromContext(ctx)
			s.handleRequest(w, r, route, parameters, ctx, responseHandler)
		} else {
			s.handlePassThrough(w, r)
		}
	})
}

func (s *Server) CreateHandler() http.Handler {
	return s.rootHandler(s.AroundRequest(s.requestHandler()))
}

func (s *Server) createResponseHandler() http.Handler {
	handler := withCombinedFragments(s)
	handler = withDefaultErrorHandler(handler)
	handler = s.AroundResponse(handler)
	handler = multiplexer.WithDefaultHeaders(handler)

	return handler
}

func (s *Server) newRequest() *multiplexer.Request {
	req := multiplexer.NewRequest(s.MultiplexerTripper, multiplexer.WithNotifier(s.Notifier))
	req.SecretFilter = s.SecretFilter
	req.Timeout = s.ProxyTimeout
	return req
}

func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request, route *Route, parameters map[string]string, ctx context.Context, handler http.Handler) {
	startTime := time.Now()
	req := s.newRequest()
	req.HmacSecret = s.HmacSecret

	for _, f := range route.FragmentsToRequest() {
		query := url.Values{}

		for name, values := range r.URL.Query() {
			if query.Get(name) == "" {
				for _, value := range values {
					query.Add(name, value)
				}
			}
		}

		dynamicParts := route.dynamicPartsFromRequest(r.URL.EscapedPath())
		requestable, err := f.Requestable(s.targetURL, dynamicParts, query)
		if len(r.URL.Query()) > 0 {
			requestable.RequestURL.RawQuery = query.Encode()
		}

		if err != nil {
			// This can be caused due to invalid encoding
			panic(err)
		}
		req.WithRequestable(requestable)
	}

	req.WithHeadersFromRequest(r)
	req.Header.Set(HeaderViewProxyOriginalPath, r.URL.RequestURI())
	results, err := req.Do(ctx)

	handlerCtx := context.WithValue(r.Context(), startTimeKey{}, startTime)
	handlerCtx = multiplexer.ContextWithResults(handlerCtx, results, err)
	handler.ServeHTTP(w, r.WithContext(handlerCtx))
}

func (s *Server) handlePassThrough(w http.ResponseWriter, r *http.Request) {
	if s.passThrough {
		s.Notifier.Emit(EventProxy, context.Background(), func(ctx context.Context) {
			s.reverseProxy.ServeHTTP(w, r)
		})
	} else {
		w.WriteHeader(404)
		w.Write([]byte("404 not found"))
	}
}

func RouteFromContext(ctx context.Context) *Route {
	if ctx == nil {
		return nil
	}

	if route := ctx.Value(routeContextKey{}); route != nil {
		return route.(*Route)
	}
	return nil
}

func ParametersFromContext(ctx context.Context) map[string]string {
	if ctx == nil {
		return nil
	}

	if parameters := ctx.Value(parametersContextKey{}); parameters != nil {
		return parameters.(map[string]string)
	}
	return nil
}

func startTimeFromContext(ctx context.Context) time.Time {
	if ctx == nil {
		return time.Time{}
	}

	if startTime := ctx.Value(startTimeKey{}); startTime != nil {
		return startTime.(time.Time)
	}
	return time.Time{}
}

func FragmentRouteFromContext(ctx context.Context) *fragment.Definition {
	requestable := multiplexer.RequestableFromContext(ctx)

	if requestable == nil {
		return nil
	}

	if fragment, ok := requestable.(*fragment.Request); ok {
		return fragment.Definition
	}

	return nil
}

func (s *Server) ListenAndServe() error {
	return s.configureServer(func() error {
		s.Logger.Printf("Listening on %v", s.Addr)
		return s.httpServer.ListenAndServe()
	})
}

func (s *Server) Serve(listener net.Listener) error {
	return s.configureServer(func() error {
		s.Logger.Printf("Listening on %v", listener.Addr())
		return s.httpServer.Serve(listener)
	})
}

func (s *Server) configureServer(serveFn func() error) error {
	s.httpServer = &http.Server{
		Addr:           s.Addr,
		Handler:        s.CreateHandler(),
		ReadTimeout:    s.ReadTimeout,
		WriteTimeout:   s.WriteTimeout,
		MaxHeaderBytes: 1 << 20,
	}

	return serveFn()
}

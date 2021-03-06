package logging

import (
	"net/http"
	"time"

	"github.com/blakewilliams/viewproxy"
	"github.com/blakewilliams/viewproxy/pkg/multiplexer"
	secretfilter "github.com/blakewilliams/viewproxy/pkg/secretfilter"
)

type logger interface {
	Print(v ...interface{})
	Printf(format string, v ...interface{})
}

type ResponseWrapper struct {
	responseWriter http.ResponseWriter
	StatusCode     int
}

func (rw *ResponseWrapper) Header() http.Header {
	return rw.responseWriter.Header()
}

func (rw *ResponseWrapper) Write(p []byte) (int, error) {
	return rw.responseWriter.Write(p)
}

func (rw *ResponseWrapper) WriteHeader(statusCode int) {
	rw.StatusCode = statusCode
	rw.responseWriter.WriteHeader(statusCode)
}

func Middleware(server *viewproxy.Server, l logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			route := viewproxy.RouteFromContext(r.Context())

			if route != nil {
				l.Printf("Handling %s", r.URL.Path)
			} else if server.PassThroughEnabled() {
				l.Printf("Proxying %s", r.URL.Path)
			} else {
				l.Printf("Proxying is disabled and no route matches %s", r.URL.Path)
			}

			wrapper := &ResponseWrapper{responseWriter: w, StatusCode: 200} // use default 200 to initialize
			next.ServeHTTP(wrapper, r)

			duration := time.Since(start)

			if route != nil {
				l.Printf("Rendered %d in %dms for %s", wrapper.StatusCode, duration.Milliseconds(), r.URL.Path)
			} else if server.PassThroughEnabled() {
				l.Printf("Proxied %d in %dms for %s", wrapper.StatusCode, duration.Milliseconds(), r.URL.Path)
			}
		})
	}
}

type logTripper struct {
	logger       logger
	secretFilter secretfilter.Filter
	tripper      multiplexer.Tripper
}

func NewLogTripper(l logger, sf secretfilter.Filter, tripper multiplexer.Tripper) multiplexer.Tripper {
	return &logTripper{logger: l, secretFilter: sf, tripper: tripper}
}

func (t *logTripper) Request(r *http.Request) (*http.Response, error) {
	start := time.Now()
	res, err := t.tripper.Request(r)
	duration := time.Since(start)
	requestable := multiplexer.RequestableFromContext(r.Context())

	if err != nil {
		if requestable != nil {
			// TODO fragment.URL is full path
			safeUrl := t.secretFilter.FilterURLString(requestable.URL())
			t.logger.Printf("Fragment exception in %dms for %s\nerror: %s", duration.Milliseconds(), safeUrl, err)
		} else {
			safeUrl := t.secretFilter.FilterURL(r.URL)
			t.logger.Printf("Proxy exception in %dms for %s\nerror: %s", duration.Milliseconds(), safeUrl, err)
		}
		return nil, err
	}

	// If fragment is nil, we are proxying
	if requestable != nil {
		// TODO fragment.URL is full path
		safeUrl := t.secretFilter.FilterURLString(requestable.URL())
		t.logger.Printf("Fragment %d in %dms for %s", res.StatusCode, duration.Milliseconds(), safeUrl)
	} else {
		safeUrl := t.secretFilter.FilterURL(r.URL)
		t.logger.Printf("Proxy request %d in %dms for %s", res.StatusCode, duration.Milliseconds(), safeUrl)
	}

	return res, err
}

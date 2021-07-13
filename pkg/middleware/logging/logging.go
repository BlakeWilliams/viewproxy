package logging

import (
	"fmt"
	"net/http"
	"time"

	"github.com/blakewilliams/viewproxy"
	"github.com/blakewilliams/viewproxy/pkg/multiplexer"
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
			} else if server.PassThrough {
				l.Printf("Proxying %s", r.URL.Path)
			} else {
				l.Printf("Proxying is disabled and no route matches %s", r.URL.Path)
			}

			wrapper := &ResponseWrapper{responseWriter: w, StatusCode: 200} // use default 200 to initialize
			next.ServeHTTP(wrapper, r)

			duration := time.Since(start)

			if route != nil {
				l.Printf("Rendered %d in %dms for %s", wrapper.StatusCode, duration.Milliseconds(), r.URL.Path)
			} else if server.PassThrough {
				l.Printf("Proxied %d in %dms for %s", wrapper.StatusCode, duration.Milliseconds(), r.URL.Path)
			}
		})
	}
}

type logTripper struct {
	logger  logger
	tripper multiplexer.Tripper
}

func NewLogTripper(l logger, tripper multiplexer.Tripper) multiplexer.Tripper {
	return &logTripper{logger: l, tripper: tripper}
}

func (t *logTripper) Request(r *http.Request) (*http.Response, error) {
	start := time.Now()
	res, err := t.tripper.Request(r)
	duration := time.Since(start)
	fragment := viewproxy.FragmentFromContext(r.Context())

	if err != nil {
		if fragment != nil {
			fmt.Println(err)
			t.logger.Printf("Fragment exception in %dms for %s\nerror: %s", duration.Milliseconds(), fragment.Url, err)
		} else {
			t.logger.Printf("Proxy exception in %dms for %s\nerror: %s", duration.Milliseconds(), r.URL, err)
		}
		return nil, err
	}

	// If fragment is nil, we are proxying
	if fragment != nil {
		t.logger.Printf("Fragment %d in %dms for %s", res.StatusCode, duration.Milliseconds(), fragment.Url)
	} else {
		t.logger.Printf("Proxy request %d in %dms for %s", res.StatusCode, duration.Milliseconds(), r.URL)
	}

	return res, err
}

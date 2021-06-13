package multiplexer

import (
	"errors"
	"net/http"
	"time"
)

var ErrNotFound = errors.New("not found")
var ErrNon2xx = errors.New("status code not in 2xx range")

type Result struct {
	Url          string
	Duration     time.Duration
	HttpResponse *http.Response
	Body         []byte
	StatusCode   int
}

func (r *Result) Header() http.Header {
	return r.HttpResponse.Header
}

func (r *Result) HeadersWithoutProxyHeaders() http.Header {
	headers := make(http.Header)

	for name, values := range r.Header() {
		headers[name] = values
	}

	for _, hopByHopHeader := range HopByHopHeaders {
		headers.Del(hopByHopHeader)
	}

	return headers
}

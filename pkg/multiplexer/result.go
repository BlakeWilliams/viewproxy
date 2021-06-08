package multiplexer

import (
	"errors"
	"net/http"
	"time"
)

var NotFoundErr = errors.New("Not found")
var Non2xxErr = errors.New("Status code not in 2xx range")

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

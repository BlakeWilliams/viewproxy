package multiplexer

import (
	"fmt"
	"net/http"
	"time"
)

type ResultError struct {
	Result *Result
}

func (re *ResultError) Error() string {
	return fmt.Sprintf(
		"status: %d url: %s",
		re.Result.StatusCode,
		re.Result.Url,
	)
}

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

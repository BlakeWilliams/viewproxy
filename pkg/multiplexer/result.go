package multiplexer

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

type ResultError struct {
	Result *Result
	msg    string
}

func newResultError(req *Request, res *Result) *ResultError {
	safeUrl := req.SecretFilter.FilterURLString(res.Url)
	msg := fmt.Sprintf("status: %d url: %s", res.StatusCode, safeUrl)

	return &ResultError{Result: res, msg: msg}
}

func (re *ResultError) Error() string {
	return re.msg
}

type Result struct {
	Url          string
	Duration     time.Duration
	HttpResponse *http.Response
	Body         []byte
	StatusCode   int
	TimingLabel  string
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

type resultsContextKey struct{}

func ResultsFromContext(ctx context.Context) []*Result {
	if ctx == nil {
		return nil
	}

	if results := ctx.Value(resultsContextKey{}); results != nil {
		return results.([]*Result)
	}
	return nil
}

func ContextWithResults(ctx context.Context, results []*Result) context.Context {
	return context.WithValue(ctx, resultsContextKey{}, results)
}

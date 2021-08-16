package multiplexer

import (
	"compress/gzip"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"sort"
	"sync"
	"time"

	"github.com/blakewilliams/viewproxy/pkg/secretfilter"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type Request struct {
	ctx          context.Context
	Header       http.Header
	layoutURL    string
	fragments    []Fragment
	Timeout      time.Duration
	HmacSecret   string
	Non2xxErrors bool
	Tripper      Tripper
	SecretFilter secretfilter.Filter
}

func NewRequest(tripper Tripper) *Request {
	return &Request{
		ctx:          context.TODO(),
		layoutURL:    "",
		fragments:    []Fragment{},
		Timeout:      time.Duration(10) * time.Second,
		HmacSecret:   "",
		Non2xxErrors: true,
		Header:       http.Header{},
		Tripper:      tripper,
	}
}

func (r *Request) WithHeadersFromRequest(req *http.Request) {
	for key, values := range HeadersFromRequest(req) {
		for _, value := range values {
			r.Header.Add(key, value)
		}
	}
}

func (r *Request) WithFragment(fragmentURL string, metadata map[string]string, timingLabel string) {
	r.fragments = append(r.fragments, Fragment{Url: fragmentURL, Metadata: metadata, timingLabel: timingLabel})
}

func (r *Request) DoSingle(ctx context.Context, method string, url string, body io.ReadCloser) (*Result, error) {
	return r.fetchUrl(ctx, method, url, r.Header, body)
}

func (r *Request) Do(ctx context.Context) ([]*Result, error) {
	tracer := otel.Tracer("multiplexer")
	var span trace.Span
	ctx, span = tracer.Start(ctx, "fetch_urls")
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()

	wg := sync.WaitGroup{}
	errCh := make(chan error)
	resultsCh := make(chan *Result, len(r.fragments))

	for _, f := range r.fragments {
		wg.Add(1)
		ctx = context.WithValue(ctx, FragmentContextKey{}, f)

		go func(ctx context.Context, f Fragment, resultsCh chan *Result, wg *sync.WaitGroup) {
			defer wg.Done()
			var span trace.Span
			ctx, span = tracer.Start(ctx, "fetch_url")
			for key, value := range f.Metadata {
				span.SetAttributes(attribute.KeyValue{
					Key:   attribute.Key(key),
					Value: attribute.StringValue(value),
				})
			}
			defer span.End()

			headersForRequest := r.Header
			if r.HmacSecret != "" {
				headersForRequest = r.headersWithHmac(f.Url)
			}

			result, err := r.fetchUrl(ctx, "GET", f.Url, headersForRequest, nil)

			if err != nil {
				errCh <- err
			} else {
				result.TimingLabel = f.timingLabel
			}

			resultsCh <- result
		}(ctx, f, resultsCh, &wg)
	}

	// wait for all responses to complete
	done := make(chan struct{})
	go (func(wg *sync.WaitGroup) {
		defer close(done)
		wg.Wait()
	})(&wg)

	select {
	case err := <-errCh:
		cancel()
		return make([]*Result, 0), err
	case <-done:
		results := make([]*Result, len(r.fragments))

		for i := 0; i < len(r.fragments); i++ {
			results[i] = <-resultsCh
		}

		sort.SliceStable(results, func(i int, j int) bool {
			return indexOfResult(r.fragments, results[i]) < indexOfResult(r.fragments, results[j])
		})

		return results, nil
	case <-ctx.Done():
		return make([]*Result, 0), ctx.Err()
	}
}

func (r *Request) fetchUrl(ctx context.Context, method string, url string, headers http.Header, body io.ReadCloser) (*Result, error) {
	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}

	for name, values := range headers {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}

	resp, err := r.Tripper.Request(req)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	duration := time.Since(start)

	var responseBody []byte

	if resp.Header.Get("Content-Encoding") == "gzip" {
		gzipReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		defer gzipReader.Close()

		responseBody, err = ioutil.ReadAll(gzipReader)
	} else {
		responseBody, err = ioutil.ReadAll(resp.Body)
	}

	if err != nil {
		return nil, err
	}

	result := &Result{
		Url:          url,
		Duration:     duration,
		HttpResponse: resp,
		Body:         responseBody,
		StatusCode:   resp.StatusCode,
	}

	if r.Non2xxErrors && (resp.StatusCode < 200 || resp.StatusCode > 299) {
		return nil, newResultError(r, result)
	}

	return result, nil
}

func (r *Request) headersWithHmac(url string) http.Header {
	newHeaders := http.Header{}
	for name, value := range r.Header {
		newHeaders[name] = value
	}

	timestamp := fmt.Sprintf("%d", time.Now().Unix())

	mac := hmac.New(sha256.New, []byte(r.HmacSecret))
	mac.Write(
		[]byte(fmt.Sprintf("%s,%s", pathFromFullUrl(url), timestamp)),
	)

	newHeaders.Set("Authorization", hex.EncodeToString(mac.Sum(nil)))
	newHeaders.Set("X-Authorization-Time", timestamp)

	return newHeaders
}

func pathFromFullUrl(fullUrl string) string {
	targetUrl, _ := url.Parse(fullUrl)

	if targetUrl.RawQuery != "" {
		return fmt.Sprintf("%s?%s", targetUrl.Path, targetUrl.RawQuery)
	} else {
		return targetUrl.Path
	}
}

func indexOfResult(fragments []Fragment, result *Result) int {
	for i, fragment := range fragments {
		if fragment.Url == result.Url {
			return i
		}
	}

	return -1
}

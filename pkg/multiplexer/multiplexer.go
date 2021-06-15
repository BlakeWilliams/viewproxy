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
)

func Fetch(ctx context.Context, urls []string, headers http.Header, timeout time.Duration, hmacSecret string) ([]*Result, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	wg := sync.WaitGroup{}
	errCh := make(chan error)
	resultsCh := make(chan *Result, len(urls))

	for _, url := range urls {
		wg.Add(1)
		go func(ctx context.Context, url string, resultsCh chan *Result, wg *sync.WaitGroup) {
			defer wg.Done()

			headersForRequest := headers
			if hmacSecret != "" {
				headersForRequest = headersWithHmac(headers, hmacSecret, url)
			}

			result, err := FetchUrl(ctx, url, headersForRequest)

			if err != nil {
				errCh <- err
			}

			resultsCh <- result
		}(ctx, url, resultsCh, &wg)
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
		results := make([]*Result, len(urls))

		for i := 0; i < len(urls); i++ {
			results[i] = <-resultsCh
		}

		sort.SliceStable(results, func(i int, j int) bool {
			return indexOfResult(urls, results[i]) < indexOfResult(urls, results[j])
		})

		return results, nil
	case <-ctx.Done():
		return make([]*Result, 0), ctx.Err()
	}
}

func headersWithHmac(headers http.Header, secret string, url string) http.Header {
	newHeaders := http.Header{}
	for name, value := range headers {
		newHeaders[name] = value
	}

	timestamp := fmt.Sprintf("%d", time.Now().Unix())

	mac := hmac.New(sha256.New, []byte(secret))
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

func indexOfResult(urls []string, result *Result) int {
	for i, url := range urls {
		if url == result.Url {
			return i
		}
	}

	return -1
}

func FetchUrlWithoutStatusCodeCheck(ctx context.Context, method string, url string, headers http.Header, body io.ReadCloser) (*Result, error) {
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

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)

	if err != nil {
		return nil, err
	}

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

	return &Result{
		Url:          url,
		Duration:     duration,
		HttpResponse: resp,
		Body:         responseBody,
		StatusCode:   resp.StatusCode,
	}, nil
}

func FetchUrl(ctx context.Context, url string, headers http.Header) (*Result, error) {
	result, err := FetchUrlWithoutStatusCodeCheck(ctx, http.MethodGet, url, headers, nil)

	if err != nil {
		return nil, err
	}

	// 404 is a failure, we should cancel the other requests
	if result.StatusCode == 404 {
		return nil, fmt.Errorf("URL %s: %w", url, NotFoundErr)
	}

	// Any non 2xx status code should be considered an error
	if !(result.StatusCode >= 200 && result.StatusCode <= 299) {
		return nil, fmt.Errorf("Status %d for URL %s: %w", result.StatusCode, url, Non2xxErr)
	}

	return result, nil
}

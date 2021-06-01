package multiplexer

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"
	"sync"
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

func Fetch(ctx context.Context, urls []string, timeout time.Duration) ([]*Result, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	wg := sync.WaitGroup{}
	errCh := make(chan error)
	resultsCh := make(chan *Result, len(urls))

	for _, url := range urls {
		wg.Add(1)
		go func(ctx context.Context, url string, resultsCh chan *Result, wg *sync.WaitGroup) {
			defer wg.Done()
			result, err := fetchUrl(ctx, url)

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

func indexOfResult(urls []string, result *Result) int {
	for i, url := range urls {
		if url == result.Url {
			return i
		}
	}

	return -1
}

func fetchUrl(ctx context.Context, url string) (*Result, error) {
	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		panic(err)
	}
	resp, err := http.DefaultClient.Do(req)

	if err != nil {
		return nil, err
	}

	duration := time.Since(start)

	body, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		return nil, err
	}

	// 404 is a failure, we should cancel the other requests
	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("URL %s: %w", url, NotFoundErr)
	}

	// Any non 2xx status code should be considered an error
	if !(resp.StatusCode >= 200 && resp.StatusCode <= 299) {
		return nil, fmt.Errorf("Status %d for URL %s: %w", resp.StatusCode, url, Non2xxErr)
	}

	// TODO handle errors
	return &Result{
		Url:          url,
		Duration:     duration,
		HttpResponse: resp,
		Body:         body,
		StatusCode:   resp.StatusCode,
	}, nil
}

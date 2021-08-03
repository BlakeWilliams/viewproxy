package multiplexer

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var defaultTimeout = time.Duration(5) * time.Second

func TestRequestDoReturnsMultipleResponsesInOrder(t *testing.T) {
	server := startServer()
	urls := []string{"http://localhost:9990?fragment=header", "http://localhost:9990?fragment=footer"}

	r := NewRequest(NewStandardTripper(&http.Client{}))
	r.WithFragment(urls[0], make(map[string]string), "")
	r.WithFragment(urls[1], make(map[string]string), "")
	r.Timeout = defaultTimeout
	results, err := r.Do(context.TODO())

	assert.Nil(t, err)

	assert.Equal(t, 2, len(results), "Expected 2 results")

	assert.Equal(t, 200, results[0].StatusCode)
	assert.Equal(t, "<body>", string(results[0].Body), "Expected first result body to be opening body tag")
	assert.Equal(t, urls[0], results[0].Url)
	assert.Greater(t, results[0].Duration, time.Duration(0))

	assert.Equal(t, 200, results[1].StatusCode)
	assert.Equal(t, "</body>", string(results[1].Body), "Expected last result body to be closing body tag")
	assert.Equal(t, urls[1], results[1].Url)
	assert.Greater(t, results[1].Duration, time.Duration(0))

	server.Close()
}

func TestRequestDoForwardsHeaders(t *testing.T) {
	server := startServer()
	headers := http.Header{}
	headers.Add("X-Name", "viewproxy")

	fakeHTTPRequest := &http.Request{Header: headers}

	r := NewRequest(NewStandardTripper(&http.Client{}))
	r.WithFragment("http://localhost:9990?fragment=echo_headers", make(map[string]string), "")
	r.WithHeadersFromRequest(fakeHTTPRequest)
	r.Timeout = defaultTimeout
	results, err := r.Do(context.TODO())

	assert.Nil(t, err)

	assert.Contains(t, string(results[0].Body), "X-Name:viewproxy", "Expected X-Name header to be present")

	server.Close()
}

func TestFetch404ReturnsError(t *testing.T) {
	server := startServer()

	r := NewRequest(NewStandardTripper(&http.Client{}))
	r.WithFragment("http://localhost:9990/wowomg", make(map[string]string), "")
	r.Timeout = defaultTimeout
	results, err := r.Do(context.TODO())

	var resultErr *ResultError
	assert.ErrorAs(t, err, &resultErr)
	assert.Equal(t, 404, resultErr.Result.StatusCode)
	assert.Equal(t, "http://localhost:9990/wowomg", resultErr.Result.Url)
	assert.Equal(t, 0, len(results), "Expected 0 results")

	server.Close()
}

func TestFetch500ReturnsError(t *testing.T) {
	server := startServer()
	start := time.Now()

	urls := []string{"http://localhost:9990/?fragment=oops", "http://localhost:9990?fragment=slow"}
	r := NewRequest(NewStandardTripper(&http.Client{}))
	r.WithFragment(urls[0], make(map[string]string), "")
	r.WithFragment(urls[1], make(map[string]string), "")
	results, err := r.Do(context.TODO())

	duration := time.Since(start)

	assert.Less(t, duration, time.Duration(3)*time.Second)
	var resultErr *ResultError
	assert.ErrorAs(t, err, &resultErr)
	assert.Equal(t, 500, resultErr.Result.StatusCode)
	assert.Equal(t, "http://localhost:9990/?fragment=oops", resultErr.Result.Url)
	assert.Equal(t, 0, len(results), "Expected 0 results")

	server.Close()
}

func TestFetchTimeout(t *testing.T) {
	server := startServer()
	start := time.Now()

	r := NewRequest(NewStandardTripper(&http.Client{}))
	r.WithFragment("http://localhost:9990?fragment=slow", make(map[string]string), "")
	r.Timeout = time.Duration(100) * time.Millisecond
	_, err := r.Do(context.Background())
	duration := time.Since(start)

	assert.EqualError(t, err, "context deadline exceeded")
	assert.Less(t, duration, time.Duration(120)*time.Millisecond)

	server.Close()
}

func TestCanIgnoreNon2xxErrors(t *testing.T) {
	server := startServer()

	ctx := context.Background()
	r := NewRequest(NewStandardTripper(&http.Client{}))
	r.WithFragment("http://localhost:9990?fragment=slow", make(map[string]string), "")
	r.Timeout = time.Duration(100) * time.Millisecond
	r.Non2xxErrors = false
	_, err := r.Do(context.Background())

	result, err := r.DoSingle(ctx, "get", "http://localhost:9990/?fragment=oops", nil)

	assert.Nil(t, err)
	assert.Equal(t, 500, result.StatusCode)

	server.Close()
}

func startServer() *http.Server {
	instance := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		params := r.URL.Query()
		fragment := params.Get("fragment")

		if fragment == "header" {
			w.Write([]byte("<body>"))
		} else if fragment == "body" {
			w.Write([]byte(fmt.Sprintf("hello %s", params.Get("name"))))
		} else if fragment == "footer" {
			w.Write([]byte("</body>"))
		} else if fragment == "slow" {
			time.Sleep(time.Duration(3) * time.Second)
			w.Write([]byte("</body>"))
		} else if fragment == "oops" {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("500"))
		} else if fragment == "echo_headers" {
			for name, values := range r.Header {
				for _, value := range values {
					w.Write(
						[]byte(fmt.Sprintf("%s:%s\n", name, value)),
					)
				}
			}
		} else {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("Not found"))
		}
	})

	testServer := &http.Server{Addr: "localhost:9990", Handler: instance}
	go func() {
		if err := testServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()

	return testServer
}

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

func TestFetchReturnsMultipleResults(t *testing.T) {
	server := startServer()

	urls := []string{"http://localhost:9990?fragment=header", "http://localhost:9990?fragment=footer"}
	results, err := Fetch(context.TODO(), urls, http.Header{}, defaultTimeout, "")

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

func TestFetchForwardsHeaders(t *testing.T) {
	server := startServer()
	headers := map[string][]string{
		"X-Name": {"viewproxy"},
	}

	urls := []string{"http://localhost:9990?fragment=echo_headers"}
	results, err := Fetch(context.TODO(), urls, headers, defaultTimeout, "")

	assert.Nil(t, err)

	assert.Contains(t, string(results[0].Body), "X-Name:viewproxy", "Expected X-Name header to be present")

	server.Close()
}

func TestFetch404ReturnsError(t *testing.T) {
	server := startServer()

	urls := []string{"http://localhost:9990/wowomg"}
	results, err := Fetch(context.TODO(), urls, http.Header{}, defaultTimeout, "")

	assert.ErrorIs(t, err, ErrNotFound)
	assert.EqualError(t, err, "URL http://localhost:9990/wowomg: not found")
	assert.Equal(t, 0, len(results), "Expected 0 results")

	server.Close()
}

func TestFetch500ReturnsError(t *testing.T) {
	server := startServer()
	start := time.Now()

	urls := []string{"http://localhost:9990/?fragment=oops", "http://localhost:9990?fragment=slow"}
	ctx := context.Background()
	results, err := Fetch(ctx, urls, http.Header{}, defaultTimeout, "")

	duration := time.Since(start)

	assert.Less(t, duration, time.Duration(3)*time.Second)
	assert.ErrorIs(t, err, ErrNon2xx)
	assert.EqualError(t, err, "status 500 for URL http://localhost:9990/?fragment=oops: status code not in 2xx range")
	assert.Equal(t, 0, len(results), "Expected 0 results")

	server.Close()
}

func TestFetchTimeout(t *testing.T) {
	server := startServer()
	start := time.Now()

	urls := []string{"http://localhost:9990?fragment=slow"}
	_, err := Fetch(context.Background(), urls, http.Header{}, time.Duration(100)*time.Millisecond, "")
	duration := time.Since(start)

	assert.EqualError(t, err, "context deadline exceeded")
	assert.Less(t, duration, time.Duration(120)*time.Millisecond)

	server.Close()
}

func TestFetchWithoutStatusCodeCheckIgnoresStatuses(t *testing.T) {
	server := startServer()

	ctx := context.Background()
	results, err := FetchUrlWithoutStatusCodeCheck(ctx, "get", "http://localhost:9990/?fragment=oops", http.Header{}, nil)

	assert.Nil(t, err)
	assert.Equal(t, 500, results.StatusCode)

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

	testServer := &http.Server{Addr: ":9990", Handler: instance}
	go func() {
		if err := testServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()

	return testServer
}

package multiplexer

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/blakewilliams/viewproxy/pkg/secretfilter"
	"github.com/stretchr/testify/require"
)

var defaultTimeout = time.Duration(5) * time.Second

type fakeRequestable struct {
	url string
}

func (ff *fakeRequestable) URL() string                 { return ff.url }
func (ff *fakeRequestable) Metadata() map[string]string { return make(map[string]string) }
func (ff *fakeRequestable) TimingLabel() string         { return "" }
func newFakeRequestable(url string) *fakeRequestable    { return &fakeRequestable{url: url} }

var _ Requestable = &fakeRequestable{}

func TestRequestDoReturnsMultipleResponsesInOrder(t *testing.T) {
	server := startServer(t)
	urls := []string{"http://localhost:9990?fragment=header", "http://localhost:9990?fragment=footer"}

	r := NewRequest(NewStandardTripper(&http.Client{}))
	r.WithRequestable(newFakeRequestable(urls[0]))
	r.WithRequestable(newFakeRequestable(urls[1]))
	r.Timeout = defaultTimeout
	results, err := r.Do(context.TODO())

	require.Nil(t, err)

	require.Equal(t, 2, len(results), "Expected 2 results")

	require.Equal(t, 200, results[0].StatusCode)
	require.Equal(t, "<body>", string(results[0].Body), "Expected first result body to be opening body tag")
	require.Equal(t, urls[0], results[0].Url)
	require.Greater(t, results[0].Duration, time.Duration(0))

	require.Equal(t, 200, results[1].StatusCode)
	require.Equal(t, "</body>", string(results[1].Body), "Expected last result body to be closing body tag")
	require.Equal(t, urls[1], results[1].Url)
	require.Greater(t, results[1].Duration, time.Duration(0))

	server.Close()
}

func TestRequestDoForwardsHeaders(t *testing.T) {
	server := startServer(t)
	headers := http.Header{}
	headers.Add("X-Name", "viewproxy")

	fakeHTTPRequest := &http.Request{Header: headers}

	r := NewRequest(NewStandardTripper(&http.Client{}))
	r.WithRequestable(newFakeRequestable("http://localhost:9990?fragment=echo_headers"))
	r.WithHeadersFromRequest(fakeHTTPRequest)
	r.Timeout = defaultTimeout
	results, err := r.Do(context.TODO())

	require.Nil(t, err)

	require.Contains(t, string(results[0].Body), "X-Name:viewproxy", "Expected X-Name header to be present")

	server.Close()
}

func TestFetch404ReturnsError(t *testing.T) {
	server := startServer(t)

	r := NewRequest(NewStandardTripper(&http.Client{}))
	r.SecretFilter = secretfilter.New()
	r.WithRequestable(newFakeRequestable("http://localhost:9990/wowomg"))
	r.Timeout = defaultTimeout
	results, err := r.Do(context.TODO())

	var resultErr *ResultError
	require.ErrorAs(t, err, &resultErr)
	require.Equal(t, 404, resultErr.Result.StatusCode)
	require.Equal(t, "http://localhost:9990/wowomg", resultErr.Result.Url)
	require.Equal(t, 0, len(results), "Expected 0 results")

	server.Close()
}

func TestResultErrorMessagesFilterUrls(t *testing.T) {
	server := startServer(t)

	r := NewRequest(NewStandardTripper(&http.Client{}))
	r.SecretFilter = secretfilter.New()
	r.WithRequestable(newFakeRequestable("http://localhost:9990/wowomg?foo=bar"))
	r.Timeout = defaultTimeout
	_, err := r.Do(context.TODO())

	var resultErr *ResultError
	require.ErrorAs(t, err, &resultErr)
	require.Equal(t, "status: 404 url: http://localhost:9990/wowomg?foo=FILTERED", resultErr.Error())

	server.Close()
}

func TestFetch500ReturnsError(t *testing.T) {
	server := startServer(t)
	start := time.Now()

	urls := []string{"http://localhost:9990/?fragment=oops", "http://localhost:9990?fragment=slow"}
	r := NewRequest(NewStandardTripper(&http.Client{}))
	r.SecretFilter = secretfilter.New()
	r.WithRequestable(newFakeRequestable(urls[0]))
	r.WithRequestable(newFakeRequestable(urls[1]))
	results, err := r.Do(context.TODO())

	duration := time.Since(start)

	require.Less(t, duration, time.Duration(3)*time.Second)
	var resultErr *ResultError
	require.ErrorAs(t, err, &resultErr)
	require.Equal(t, 500, resultErr.Result.StatusCode)
	require.Equal(t, "http://localhost:9990/?fragment=oops", resultErr.Result.Url)
	require.Equal(t, 0, len(results), "Expected 0 results")

	server.Close()
}

func TestFetchTimeout(t *testing.T) {
	server := startServer(t)
	start := time.Now()

	r := NewRequest(NewStandardTripper(&http.Client{}))
	r.WithRequestable(newFakeRequestable("http://localhost:9990?fragment=slow"))
	r.Timeout = time.Duration(100) * time.Millisecond
	_, err := r.Do(context.Background())
	duration := time.Since(start)

	require.EqualError(t, err, "multiplexer timed out: context deadline exceeded")
	require.Less(t, duration, time.Duration(120)*time.Millisecond)

	server.Close()
}

func TestCanIgnoreNon2xxErrors(t *testing.T) {
	server := startServer(t)

	ctx := context.Background()
	r := NewRequest(NewStandardTripper(&http.Client{}))
	r.WithRequestable(newFakeRequestable("http://localhost:9990?frgagment=slow"))
	r.Timeout = time.Duration(100) * time.Millisecond
	r.Non2xxErrors = false
	_, err := r.Do(context.Background())

	result, err := r.DoSingle(ctx, "get", "http://localhost:9990/?fragment=oops", nil)

	require.Nil(t, err)
	require.Equal(t, 500, result.StatusCode)

	server.Close()
}

func startServer(t *testing.T) *http.Server {
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

	listener, err := net.Listen("tcp", "localhost:9990")
	require.NoError(t, err)

	testServer := &http.Server{Handler: instance}
	go func() {
		if err := testServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			require.NoError(t, err)
		}
	}()

	return testServer
}

func TestTimeoutError(t *testing.T) {
	originalError := errors.New("omg")
	err := newTimeoutError(originalError)

	require.Equal(t, "multiplexer timed out: omg", err.Error())
	require.Equal(t, originalError, err.Unwrap())
}

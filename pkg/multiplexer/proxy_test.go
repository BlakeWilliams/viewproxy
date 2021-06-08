package multiplexer

import (
	"context"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProxyRequest(t *testing.T) {
	done := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(done)

		assert.Equal(t, "", r.Header.Get("Keep-Alive"), "Expected Keep-Alive to be filtered")
		assert.Equal(t, "0.0.0.0", r.Header.Get("X-Forwarded-For"))
	}))

	headers := make(http.Header)
	headers.Set("Keep-Alive", "1000")

	fakeReq := http.Request{
		RemoteAddr: "0.0.0.0:3005",
		Header:     headers,
	}

	result, err := ProxyRequest(context.TODO(), server.URL, &fakeReq)

	assert.Nil(t, err)
	assert.Equal(t, "", result.Header().Get("Keep-Alive"), "Expected Keep-Alive to be filtered")

	select {
	case <-done:
		server.Close()
	}
}

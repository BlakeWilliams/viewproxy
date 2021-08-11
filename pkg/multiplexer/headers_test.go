package multiplexer

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPreservesForwardedHeaders(t *testing.T) {
	headers := http.Header{}
	headers.Add("X-Forwarded-For", "1.2.3.4")
	headers.Add("X-Forwarded-Host", "example.com")
	headers.Add("X-Forwarded-Proto", "httpz")
	fakeHTTPRequest := &http.Request{Header: headers}
	fakeHTTPRequest.RemoteAddr = "1.3.5.7"

	newHeaders := HeadersFromRequest(fakeHTTPRequest)

	// append X-Forwarded-For
	assert.Equal(t, "1.2.3.4, 1.3.5.7", newHeaders.Get("X-Forwarded-For"))

	// preserve X-Forwarded-Host and X-Forwarded-Proto
	assert.Equal(t, "example.com", newHeaders.Get("X-Forwarded-Host"))
	assert.Equal(t, "httpz", newHeaders.Get("X-Forwarded-Proto"))
}

func TestSetsDefaultForwardedHeaders(t *testing.T) {
	fakeHTTPRequest := &http.Request{}
	fakeHTTPRequest.Proto = "httpz"
	fakeHTTPRequest.Host = "example.com"
	fakeHTTPRequest.RemoteAddr = "1.3.5.7"

	newHeaders := HeadersFromRequest(fakeHTTPRequest)

	// append X-Forwarded-For
	assert.Equal(t, "1.3.5.7", newHeaders.Get("X-Forwarded-For"))

	// set default X-Forwarded-Host and X-Forwarded-Proto
	assert.Equal(t, "example.com", newHeaders.Get("X-Forwarded-Host"))
	assert.Equal(t, "httpz", newHeaders.Get("X-Forwarded-Proto"))
}

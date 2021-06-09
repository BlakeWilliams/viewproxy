package multiplexer

import (
	"context"
	"fmt"
	"net"
	"net/http"
)

// Hop-by-hop headers defined here: https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers
var HopByHopHeaders []string = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"TE",
	"Trailers",
	"Transfer-Encoding",
	"Upgrade",
}

func ProxyRequest(ctx context.Context, targetUrl string, req *http.Request) (*Result, error) {
	headers := generateProxyRequestHeaders(req.Header)

	host, _, err := net.SplitHostPort(req.RemoteAddr)

	if err != nil {
		return nil, err
	}

	if val := headers.Get("X-Forwarded-For"); val != "" {
		newHeader := fmt.Sprintf("%s, %s", val, host)
		headers.Set("X-Forwarded-For", newHeader)
	} else {
		headers.Set("X-Forwarded-For", host)
	}

	// go strips the host header for some reason
	// https://github.com/golang/go/blob/master/src/net/http/server.go#L999
	headers.Set("Host", req.Host)
	headers.Set("X-Forwarded-Host", req.Host)

	// TODO handle timeouts or maybe rely on target?
	result, err := fetchUrlWithoutStatusCodeCheck(context.TODO(), req.Method, targetUrl, headers, req.Body)

	if err != nil {
		return nil, err
	}

	return result, nil
}

func generateProxyRequestHeaders(headers http.Header) http.Header {
	newHeaders := make(http.Header)

	// TODO remove headers listed in the Connection header

	for name, values := range headers {
		newHeaders[name] = values
	}

	for _, hopByHopHeader := range HopByHopHeaders {
		newHeaders.Del(hopByHopHeader)
	}

	return newHeaders
}

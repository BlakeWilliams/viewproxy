package multiplexer

import (
	"context"
	"fmt"
	"net"
	"net/http"
)

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
		newHeader := fmt.Sprintf("%s,%s", val, host)
		headers.Set("X-Forwarded-For", newHeader)
	} else {
		headers.Set("X-Forwarded-For", host)
	}

	headers.Set("X-Forwarded-Proto", req.Proto)

	// TODO handle timeouts or maybe rely on target?
	result, err := fetchUrlWithoutStatusCodeCheck(context.TODO(), targetUrl, headers)

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

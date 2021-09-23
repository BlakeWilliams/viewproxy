package multiplexer

import (
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

// TODO remove headers listed in the Connection header
func HeadersFromRequest(req *http.Request) http.Header {
	newHeaders := make(http.Header)

	for name, values := range req.Header {
		newHeaders[name] = values
	}

	for _, hopByHopHeader := range HopByHopHeaders {
		newHeaders.Del(hopByHopHeader)
	}

	// Set Forwarded-For headers since we act as a proxy
	host := forwardedForFromRequest(req)
	if val := newHeaders.Get("X-Forwarded-For"); val != "" {
		newHeader := fmt.Sprintf("%s, %s", val, host)
		newHeaders.Set("X-Forwarded-For", newHeader)
	} else {
		newHeaders.Set("X-Forwarded-For", host)
	}

	// go strips the host header for some reason
	// https://github.com/golang/go/blob/master/src/net/http/server.go#L999
	newHeaders.Set("Host", req.Host)

	if val := newHeaders.Get("X-Forwarded-Host"); val == "" {
		newHeaders.Set("X-Forwarded-Host", req.Host)
	}
	if val := newHeaders.Get("X-Forwarded-Proto"); val == "" {
		newHeaders.Set("X-Forwarded-Proto", req.Proto)
	}

	return newHeaders
}

func forwardedForFromRequest(req *http.Request) string {
	host, _, err := net.SplitHostPort(req.RemoteAddr)

	if err != nil {
		return req.RemoteAddr
	}

	return host
}

func WithDefaultHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		results := ResultsFromContext(r.Context())

		if results != nil && len(results.Results()) > 0 {
			headers := results.Results()[0].HeadersWithoutProxyHeaders()
			for name, values := range headers {
				for _, value := range values {
					rw.Header().Add(name, value)
				}
			}

			rw.Header().Del("Content-Length")
		}

		next.ServeHTTP(rw, r)
	})
}

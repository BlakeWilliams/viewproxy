package viewproxy

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"strconv"

	"github.com/blakewilliams/viewproxy/pkg/multiplexer"
)

type responseBuilder struct {
	writer     http.ResponseWriter
	server     Server
	body       []byte
	StatusCode int
}

func newResponseBuilder(server Server, w http.ResponseWriter) *responseBuilder {
	return &responseBuilder{server: server, writer: w, StatusCode: 200}
}

func (rb *responseBuilder) SetLayout(result *multiplexer.Result) {
	rb.body = result.Body
}

func (rb *responseBuilder) SetHeaders(headers http.Header, results []*multiplexer.Result) {
	for name, values := range headers {
		if !rb.server.ignoreHeaders[http.CanonicalHeaderKey(name)] {
			for _, value := range values {
				rb.writer.Header().Add(name, value)
			}
		}
	}

	if len(results) > 1 {
		multiplexer.SetCombinedServerTimingHeader(results, rb.writer)
	}
}

func (rb *responseBuilder) SetFragments(results []*multiplexer.Result) {
	var contentHtml []byte
	var pageTitle string

	for _, result := range results {
		contentHtml = append(contentHtml, result.Body...)

		if result.HttpResponse.Header.Get("X-View-Proxy-Title") != "" {
			pageTitle = result.HttpResponse.Header.Get("X-View-Proxy-Title")
		}
	}

	if pageTitle == "" {
		pageTitle = rb.server.DefaultPageTitle
	}

	if len(rb.body) == 0 {
		rb.body = contentHtml
	} else {
		outputHtml := bytes.Replace(rb.body, []byte("<view-proxy-content></view-proxy-content>"), contentHtml, 1)
		outputHtml = bytes.Replace(outputHtml, []byte("{{{VIEW_PROXY_PAGE_TITLE}}}"), []byte(pageTitle), 1)

		rb.body = outputHtml
	}
}

func (rb *responseBuilder) SetDuration(duration int64) {
	outputHtml := bytes.Replace(rb.body, []byte("<view-proxy-timing></view-proxy-timing>"), []byte(strconv.FormatInt(duration, 10)), 1)
	rb.body = outputHtml
}

func (rb *responseBuilder) Write() {
	rb.writer.WriteHeader(rb.StatusCode)

	if rb.writer.Header().Get("Content-Encoding") == "gzip" {
		var b bytes.Buffer
		gzipWriter := gzip.NewWriter(&b)

		_, err := gzipWriter.Write(rb.body)
		if err != nil {
			rb.server.Logger.Printf("Could not write to gzip buffer: %s", err)
		}

		gzipWriter.Close()
		if err != nil {
			rb.server.Logger.Printf("Could not closeto gzip buffer: %s", err)
		}

		rb.writer.Write(b.Bytes())
	} else {
		rb.writer.Write(rb.body)
	}
}

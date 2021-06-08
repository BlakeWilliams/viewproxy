package viewproxy

import (
	"bytes"
	"github.com/blakewilliams/viewproxy/pkg/multiplexer"
	"net/http"
	"strings"
)

type responseBuilder struct {
	writer http.ResponseWriter
	server Server
	body   []byte
}

func newResponseBuilder(server Server, w http.ResponseWriter) *responseBuilder {
	return &responseBuilder{server: server, writer: w}
}

func (rb *responseBuilder) SetLayout(result *multiplexer.Result) {
	rb.body = result.Body

	for name, values := range result.HttpResponse.Header {
		if _, ok := rb.server.ignoreHeaders[strings.ToLower(name)]; !ok {
			for _, value := range values {
				rb.writer.Header().Add(name, value)
			}
		}
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

	outputHtml := bytes.Replace(rb.body, []byte("{{{VIEW_PROXY_CONTENT}}}"), contentHtml, 1)
	outputHtml = bytes.Replace(outputHtml, []byte("{{{VIEW_PROXY_PAGE_TITLE}}}"), []byte(pageTitle), 1)

	rb.body = outputHtml
}

func (rb *responseBuilder) Write() {
	rb.writer.Write(rb.body)
}

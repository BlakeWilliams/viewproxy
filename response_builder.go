package viewproxy

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"net/http"
	"strconv"
	"time"

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

func (rb *responseBuilder) SetFragments(route *Route, results []*multiplexer.Result) {
	resultMap := mapResultsToFragmentKey(route, results)
	rb.body = stitch(route.structure, resultMap)
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

func withDefaultErrorHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		results := multiplexer.ResultsFromContext(r.Context())

		if results != nil && results.Error() != nil {
			rw.WriteHeader(http.StatusInternalServerError)
			rw.Write([]byte("500 internal server error"))
		} else {
			next.ServeHTTP(rw, r)
		}
	})
}

func withCombinedFragments(s *Server) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		route := RouteFromContext(r.Context())
		results := multiplexer.ResultsFromContext(r.Context())

		if results != nil && results.Error() == nil {
			resBuilder := newResponseBuilder(*s, rw)
			resBuilder.SetFragments(route, results.Results())
			elapsed := time.Since(startTimeFromContext(r.Context()))
			resBuilder.SetDuration(elapsed.Milliseconds())
			resBuilder.Write()
		}
	})
}

func stitch(structure stitchStructure, results map[string]*multiplexer.Result) []byte {
	childContent := make(map[string][]byte)

	for _, childBuild := range structure.DependentStructures() {
		childContent[childBuild.ReplacementID()] = stitch(childBuild, results)
	}

	self := results[structure.Key()].Body

	// handle edge fragments
	if len(childContent) == 0 {
		return self
	}

	for replacementKey, content := range childContent {
		directive := []byte(fmt.Sprintf("<viewproxy-fragment id=\"%s\"/>", replacementKey))
		self = bytes.Replace(self, directive, content, 1)
	}

	return self
}

func mapResultsToFragmentKey(route *Route, results []*multiplexer.Result) map[string]*multiplexer.Result {
	resultMap := map[string]*multiplexer.Result{}

	for i, key := range route.FragmentOrder() {
		resultMap[key] = results[i]
	}

	return resultMap
}

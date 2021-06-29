package multiplexer

import (
	"net/http"
	"strings"

	servertiming "github.com/mitchellh/go-server-timing"
)

func SetServerTimingHeader(results []*Result, writer http.ResponseWriter) {
	// Forward header directly if only one result
	if len(results) == 1 {
		value := results[0].HttpResponse.Header.Get(servertiming.HeaderKey)
		if len(value) > 0 {
			writer.Header().Set(servertiming.HeaderKey, value)
		}
		return
	}

	metrics := []*servertiming.Metric{}

	for _, result := range results {
		fragment, ok := result.metadata["timing"]

		// Skip results with no timing label
		if !ok {
			continue
		}

		resultTiming := result.HttpResponse.Header.Get(servertiming.HeaderKey)
		timings, err := servertiming.ParseHeader(resultTiming)
		if err != nil {
			continue
		}

		for _, metric := range timings.Metrics {
			if metric.Duration == 0 {
				continue
			}
			metric.Desc = fragment + " " + metric.Name
			metric.Name = fragment + "-" + metric.Name
			metrics = append(metrics, metric)
		}
	}

	if len(metrics) == 0 {
		return
	}

	segments := make([]string, 0, len(metrics))
	for _, metric := range metrics {
		segments = append(segments, metric.String())
	}

	writer.Header().Set(servertiming.HeaderKey, strings.Join(segments, ","))
}

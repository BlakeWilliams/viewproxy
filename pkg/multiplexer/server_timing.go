package multiplexer

import (
	"net/http"
	"strings"

	servertiming "github.com/mitchellh/go-server-timing"
)

func AggregateServerTimingHeaders(results []*Result, writer http.ResponseWriter) {
	metrics := []*servertiming.Metric{}

	for _, result := range results {
		// Skip results with no timing label
		if len(result.TimingLabel) == 0 {
			continue
		}

		resultTiming := result.HttpResponse.Header.Get(servertiming.HeaderKey)
		timings, err := servertiming.ParseHeader(resultTiming)
		if err != nil {
			continue
		}

		for _, metric := range timings.Metrics {
			// Ignore zero duration timings to reduce UI noise
			if metric.Duration == 0 {
				continue
			}
			metric.Desc = result.TimingLabel + " " + metric.Name
			metric.Name = result.TimingLabel + "-" + metric.Name
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
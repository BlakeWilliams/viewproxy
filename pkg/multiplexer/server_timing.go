package multiplexer

import (
	"net/http"
	"strings"

	servertiming "github.com/mitchellh/go-server-timing"
)

const resultTimingLabel = "fragment"

func WithCombinedServerTimingHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if results := ResultsFromContext(r.Context()); results != nil {
			metrics := metricsForResults(results.Results())

			if len(metrics) > 0 {
				segments := make([]string, 0, len(metrics))
				for _, metric := range metrics {
					segments = append(segments, metric.String())
				}

				rw.Header().Set(servertiming.HeaderKey, strings.Join(segments, ","))
			}
		}

		next.ServeHTTP(rw, r)
	})
}

func metricsForResults(results []*Result) []*servertiming.Metric {
	metrics := []*servertiming.Metric{}

	for _, result := range results {
		// Skip results with no timing label
		if len(result.TimingLabel) == 0 {
			continue
		}

		metrics = append(metrics, &servertiming.Metric{
			Desc:     result.TimingLabel + " " + resultTimingLabel,
			Name:     result.TimingLabel + "-" + resultTimingLabel,
			Duration: result.Duration,
		})

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

	return metrics
}

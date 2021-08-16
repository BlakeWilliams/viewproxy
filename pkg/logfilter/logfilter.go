package logfilter

import (
	"net/url"
	"strings"
)

type Filter interface {
	Allow(string)
	IsAllowed(string) bool
	FilterURL(url *url.URL) *url.URL
	FilterURLString(url string) string
	FilterQueryParams(params url.Values) url.Values
}

type mapKey struct{}

type logFilter struct {
	allowedMap map[string]mapKey
}

var _ Filter = &logFilter{}

func New() Filter {
	return &logFilter{allowedMap: make(map[string]mapKey)}
}

func (l *logFilter) Allow(key string) {
	l.allowedMap[strings.ToLower(key)] = mapKey{}
}

func (l *logFilter) IsAllowed(key string) bool {
	if _, ok := l.allowedMap[strings.ToLower(key)]; ok {
		return true
	}

	return false
}

func (l *logFilter) FilterURLString(urlString string) string {
	parsedUrl, err := url.Parse(urlString)

	if err != nil {
		return "FILTEREDINVALIDURL"
	}

	return l.FilterURL(parsedUrl).String()
}

func (l *logFilter) FilterURL(originalUrl *url.URL) *url.URL {
	clonedUrl, _ := url.Parse(originalUrl.String())

	if clonedUrl.User != nil {
		clonedUrl.User = url.UserPassword("FILTERED", "FILTERED")
	}

	filteredParams := l.FilterQueryParams(clonedUrl.Query())
	clonedUrl.RawQuery = filteredParams.Encode()

	return clonedUrl
}

func (l *logFilter) FilterQueryParams(query url.Values) url.Values {
	filteredQueryParams := make(url.Values, len(query))

	for key, values := range query {
		for _, value := range values {
			if l.IsAllowed(key) {
				filteredQueryParams.Add(key, value)
			} else {
				filteredQueryParams.Add(key, "FILTERED")
			}
		}
	}

	return filteredQueryParams
}

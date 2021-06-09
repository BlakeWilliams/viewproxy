package viewproxy

import (
	"net/url"
	"strings"
)

type Route struct {
	Parts     []string
	Layout    string
	fragments []string
}

func newRoute(path string, layout string, fragments []string) *Route {
	return &Route{
		Parts:     strings.Split(path, "/"),
		Layout:    layout,
		fragments: fragments,
	}
}

func (r *Route) matchParts(pathParts []string) bool {
	if len(r.Parts) != len(pathParts) {
		return false
	}

	for i := 0; i < len(r.Parts); i++ {
		if r.Parts[i] != pathParts[i] && !strings.HasPrefix(r.Parts[i], ":") {
			return false
		}
	}

	return true
}

func (r *Route) parametersFor(pathParts []string) map[string]string {
	parameters := make(map[string]string)

	for i := 0; i < len(r.Parts); i++ {
		if strings.HasPrefix(r.Parts[i], ":") {
			paramName := r.Parts[i][1:]
			parameters[paramName] = pathParts[i]
		}
	}

	return parameters
}

func (r *Route) fragmentsWithParameters(parameters map[string]string) []string {
	query := url.Values{}
	for name, value := range parameters {
		query.Add(name, value)
	}

	urls := make([]string, len(r.fragments)+1)
	urls[0] = fragmentToUrl(r.Layout, query)

	for i, fragment := range r.fragments {
		urls[i+1] = fragmentToUrl(fragment, query)
	}

	return urls
}

func fragmentToUrl(fragment string, parameters url.Values) string {
	// This is already parsed before constructing the url in server.go, so we ignore errors
	targetUrl, _ := url.Parse(fragment)
	targetUrl.RawQuery = parameters.Encode()

	return targetUrl.String()
}

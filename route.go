package viewproxy

import (
	"strings"

	"github.com/blakewilliams/viewproxy/pkg/fragments"
)

type Route struct {
	Path             string
	Parts            []string
	LayoutFragment   *fragments.Definition
	ContentFragments ContentFragments
	Metadata         map[string]string
}

func newRoute(path string, metadata map[string]string, layout *fragments.Definition, fragments ContentFragments) *Route {
	return &Route{
		Path:             path,
		Parts:            strings.Split(path, "/"),
		LayoutFragment:   layout,
		ContentFragments: fragments,
		Metadata:         metadata,
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

func (r *Route) FragmentsToRequest() ContentFragments {
	fragments := make(ContentFragments, len(r.ContentFragments)+1)
	fragments[0] = r.LayoutFragment

	for i, fragment := range r.ContentFragments {
		fragments[i+1] = fragment
	}
	return fragments
}

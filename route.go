package viewproxy

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/blakewilliams/viewproxy/pkg/fragment"
)

type RouteValidationError struct {
	Route    *Route
	Fragment *fragment.Definition
}

func (rve *RouteValidationError) Error() string {
	if rve.Route.HasDynamicParts() {
		return fmt.Sprintf(
			"dynamic route %s has mismatched fragment route %s",
			rve.Route.Path,
			rve.Fragment.Path,
		)
	} else {
		return fmt.Sprintf(
			"static route %s has mismatched fragment route %s",
			rve.Route.Path,
			rve.Fragment.Path,
		)
	}
}

type Route struct {
	Path             string
	Parts            []string
	LayoutFragment   *fragment.Definition
	ContentFragments fragment.Collection
	Metadata         map[string]string
}

func newRoute(path string, metadata map[string]string, layout *fragment.Definition, contentFragments fragment.Collection) *Route {
	return &Route{
		Path:             path,
		Parts:            strings.Split(path, "/"),
		LayoutFragment:   layout,
		ContentFragments: contentFragments,
		Metadata:         metadata,
	}
}

// Validates if the route and fragments have compatible dynamic route parts.
func (r *Route) Validate() error {
	// Legacy routes skip validation
	if r.Metadata["legacy"] == "true" {
		return nil
	}

	if r.HasDynamicParts() {
		dynamicParts := r.DynamicParts()

		for _, fragment := range r.FragmentsToRequest() {
			if !reflect.DeepEqual(dynamicParts, fragment.DynamicParts()) {
				return &RouteValidationError{Route: r, Fragment: fragment}
			}
		}

		return nil
	} else {
		for _, fragment := range r.FragmentsToRequest() {
			if fragment.HasDynamicParts() {
				return &RouteValidationError{Route: r, Fragment: fragment}
			}
		}

		return nil
	}
}

func (r *Route) HasDynamicParts() bool {
	for _, part := range r.Parts {
		if strings.HasPrefix(part, ":") {
			return true
		}
	}

	return false
}

func (r *Route) DynamicParts() []string {
	parts := make([]string, 0)

	for _, part := range r.Parts {
		if strings.HasPrefix(part, ":") {
			parts = append(parts, part)
		}
	}
	return parts
}

func (r *Route) DynamicPartsFromRequest(path string) map[string]string {
	dynamicParts := make(map[string]string)
	routeParts := strings.Split(path, "/")

	for i, part := range r.Parts {
		if strings.HasPrefix(part, ":") {
			dynamicParts[part] = routeParts[i]
		}
	}

	return dynamicParts
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

func (r *Route) FragmentsToRequest() fragment.Collection {
	fragments := make(fragment.Collection, len(r.ContentFragments)+1)
	fragments[0] = r.LayoutFragment

	for i, fragment := range r.ContentFragments {
		fragments[i+1] = fragment
	}
	return fragments
}

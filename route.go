package viewproxy

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/blakewilliams/viewproxy/pkg/fragment"
)

type RouteValidationError struct {
	Route    *Route
	Fragment *fragment.Definition
}

func (rve *RouteValidationError) Error() string {
	if len(rve.Route.dynamicParts) > 0 {
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
	Path         string
	Parts        []string
	dynamicParts []string
	RootFragment *fragment.Definition
	Metadata     map[string]string
}

func newRoute(path string, metadata map[string]string, root *fragment.Definition) *Route {
	route := &Route{
		Path:         path,
		Parts:        strings.Split(path, "/"),
		Metadata:     metadata,
		RootFragment: root,
	}

	dynamicParts := make([]string, 0)
	for _, part := range route.Parts {
		if strings.HasPrefix(part, ":") {
			dynamicParts = append(dynamicParts, part)
		}
	}
	route.dynamicParts = dynamicParts

	return route
}

// Validates if the route and fragments have compatible dynamic route parts.
func (r *Route) Validate() error {
	// Legacy routes skip validation
	if r.Metadata["legacy"] == "true" {
		return nil
	}

	for _, fragment := range r.FragmentsToRequest() {
		if !fragment.IgnoreValidation && !compareStringSlice(r.dynamicParts, fragment.DynamicParts()) {
			return &RouteValidationError{Route: r, Fragment: fragment}
		}
	}

	return nil
}

func compareStringSlice(first []string, other []string) bool {
	sort.Strings(first)
	sort.Strings(other)

	return reflect.DeepEqual(first, other)
}

func (r *Route) dynamicPartsFromRequest(path string) map[string]string {
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

func (r *Route) FragmentOrder() []string {
	mapping := r.RootFragment.Mapping()
	keys := make([]string, 0, len(mapping))

	for key, _ := range mapping {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	return keys
}

func (r *Route) FragmentsToRequest() []*fragment.Definition {
	mapping := r.RootFragment.Mapping()
	keys := r.FragmentOrder()

	fragments := make([]*fragment.Definition, 0, len(keys))
	for _, key := range keys {
		fragments = append(fragments, mapping[key])
	}

	return fragments
}

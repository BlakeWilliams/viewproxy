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
	// memoized version of the mapping used to stitch fragments back together
	structure *stitchStructure
	// memoized version of fragments to request
	fragmentsToRequest []*fragment.Definition
	// memoized version mapping fragment names to multiplexer.Result order
	fragmentOrder []string
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
	route.structure = stitchStructureFor(root)

	route.memoizeFragments()

	return route
}

// Validates if the route and fragments have compatible dynamic route parts.
func (r *Route) Validate() error {
	for _, fragment := range r.FragmentsToRequest() {
		if !fragment.IgnoreValidation && !compareStringSlice(r.dynamicParts, fragment.DynamicParts()) {
			return &RouteValidationError{Route: r, Fragment: fragment}
		}
	}

	return nil
}

func (r *Route) FragmentOrder() []string {
	return r.fragmentOrder
}

func (r *Route) FragmentsToRequest() []*fragment.Definition {
	return r.fragmentsToRequest
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

func (r *Route) memoizeFragments() {
	mapping := fragmentMapping(r.RootFragment)

	keys := make([]string, 0, len(mapping))

	for key := range mapping {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	r.fragmentOrder = keys

	fragments := make([]*fragment.Definition, 0, len(keys))
	for _, key := range keys {
		fragments = append(fragments, mapping[key])
	}

	r.fragmentsToRequest = fragments
}

// fragmentMapping returns a map of fragment keys and their fragments.
//
// Fragment keys consist of each parent's name separated by a `.`. The top-level
// fragment is always named root and child fragments are named after their key
// in the parent's `Children` map. e.g. `root.layout.header`
func fragmentMapping(f *fragment.Definition) map[string]*fragment.Definition {
	mapping := make(map[string]*fragment.Definition)
	mapping["root"] = f

	for name, child := range f.Children() {
		mapChildFragment("root", name, child, mapping)
	}

	return mapping
}

func mapChildFragment(prefix string, name string, f *fragment.Definition, mapping map[string]*fragment.Definition) {
	key := prefix + "." + name
	mapping[key] = f

	for name, child := range f.Children() {
		mapChildFragment(key, name, child, mapping)
	}
}

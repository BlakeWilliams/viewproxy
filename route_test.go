package viewproxy

import (
	"reflect"
	"strings"
	"testing"

	fragment "github.com/blakewilliams/viewproxy/pkg/fragment"
	"github.com/stretchr/testify/require"
)

func TestRoute_MatchParts(t *testing.T) {
	tests := map[string]struct {
		routePath   string
		providedUrl string
		want        bool
	}{
		"root":                     {routePath: "/", providedUrl: "/", want: true},
		"mismatched root route":    {routePath: "/", providedUrl: "/hello-world", want: false},
		"matching static routes":   {routePath: "/hello/world", providedUrl: "/hello/world", want: true},
		"mismatched static routes": {routePath: "/hello/world", providedUrl: "/hello/false", want: false},
		"valid dynamic route":      {routePath: "/hello/:name", providedUrl: "/hello/world", want: true},
		"invalid dynamic route":    {routePath: "/hello/:name", providedUrl: "/hello/world/wow", want: false},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			route := newRoute(test.routePath, map[string]string{}, fragment.Define(""))
			providedUrlParts := strings.Split(test.providedUrl, "/")
			got := route.matchParts(providedUrlParts)

			if got != test.want {
				t.Fatalf("expected route %s to match URL %s", test.routePath, test.providedUrl)
			}
		})
	}
}

func TestRoute_ParametersFor(t *testing.T) {
	tests := map[string]struct {
		routePath   string
		providedUrl string
		want        map[string]string
	}{
		"simple":      {routePath: "/", providedUrl: "/", want: map[string]string{}},
		"multi false": {routePath: "/hello/:name", providedUrl: "/hello/world", want: map[string]string{"name": "world"}},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			route := newRoute(test.routePath, map[string]string{}, fragment.Define(""))
			providedUrlParts := strings.Split(test.providedUrl, "/")
			got := route.parametersFor(providedUrlParts)

			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("expected route %v with URL %s to have params: %v\n got: %v", test.routePath, test.providedUrl, test.want, got)
			}
		})
	}
}

func TestRoute_Validate(t *testing.T) {
	testCases := map[string]struct {
		routePath   string
		root        *fragment.Definition
		errorString string
		valid       bool
	}{
		"static routes": {
			routePath: "/foo",
			root: fragment.Define("/foo/layout", fragment.WithChild(
				"body", fragment.Define("body"),
			)),
		},
		"dynamic route matching": {
			routePath: "/hello/:name",
			root: fragment.Define("/_viewproxy/hello/:name/layout", fragment.WithChild(
				"body", fragment.Define("/_viewproxy/hello/:name/body"),
			)),
		},
		"dynamic route matching with different order": {
			routePath: "/:greeting/:name",
			root: fragment.Define("/_viewproxy/:greeting/:name/layout", fragment.WithChild(
				"body", fragment.Define("/_viewproxy/hello/:name/:greeting/body"),
			)),
		},
		"dynamic route layout not matching": {
			routePath: "/hello/:name",
			root: fragment.Define("/_viewproxy/hello/:login/layout", fragment.WithChild(
				"body", fragment.Define("/_viewproxy/hello/:name/body"),
			)),
			errorString: "dynamic route /hello/:name has mismatched fragment route /_viewproxy/hello/:login/layout",
		},
		"dynamic route layout not matching without validation": {
			routePath: "/hello/:name",
			root: fragment.Define("/_viewproxy/hello/:login/layout", fragment.WithoutValidation(), fragment.WithChild(
				"body", fragment.Define("/_viewproxy/hello/:name/body"),
			)),
		},
		"dynamic route body not matching": {
			routePath: "/hello/:name",
			root: fragment.Define("/_viewproxy/hello/:name/layout", fragment.WithChild(
				"body", fragment.Define("/_viewproxy/hello/:login/body"),
			)),
			errorString: "dynamic route /hello/:name has mismatched fragment route /_viewproxy/hello/:login/body",
		},
		"dynamic route body not matching without validation": {
			routePath: "/hello/:name",
			root: fragment.Define("/_viewproxy/hello/:name/layout", fragment.WithChild(
				"body", fragment.Define("/_viewproxy/hello/:login/body", fragment.WithoutValidation()),
			)),
		},
		"static route with dynamic layout": {
			routePath: "/foo",
			root: fragment.Define("/_viewproxy/hello/:name/layout", fragment.WithChild(
				"body", fragment.Define("body"),
			)),
			errorString: "static route /foo has mismatched fragment route /_viewproxy/hello/:name/layout",
		},
		"static route with dynamic body": {
			routePath: "/foo",
			root: fragment.Define("/_viewproxy/foo/layout", fragment.WithChild(
				"body", fragment.Define("/_viewproxy/hello/:name/body"),
			)),
			errorString: "static route /foo has mismatched fragment route /_viewproxy/hello/:name/body",
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			route := newRoute(tc.routePath, map[string]string{}, tc.root)

			err := route.Validate()

			if tc.errorString == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tc.errorString)
			}
		})
	}
}

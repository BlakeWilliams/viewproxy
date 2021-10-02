package fragment

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/blakewilliams/viewproxy/pkg/multiplexer"
)

type Collection = []*Definition
type DefinitionOption = func(*Definition)

type Definition struct {
	Path             string
	routeParts       []string
	dynamicParts     []string
	Url              string
	Metadata         map[string]string
	IgnoreValidation bool
	children         map[string]*Definition
}

func Define(path string, options ...DefinitionOption) *Definition {
	safePath := strings.TrimPrefix(path, "/")
	definition := &Definition{
		Path:       path,
		routeParts: strings.Split(safePath, "/"),
		Metadata:   make(map[string]string),
		children:   make(map[string]*Definition),
	}

	dynamicParts := make([]string, 0)
	for _, part := range definition.routeParts {
		if strings.HasPrefix(part, ":") {
			dynamicParts = append(dynamicParts, part)
		}
	}
	definition.dynamicParts = dynamicParts

	for _, option := range options {
		option(definition)
	}

	return definition
}

func WithChild(name string, child *Definition) DefinitionOption {
	return func(definition *Definition) {
		// TODO error if overwriting?
		definition.children[name] = child
	}
}

func WithoutValidation() DefinitionOption {
	return func(definition *Definition) {
		definition.IgnoreValidation = true
	}
}

func WithMetadata(metadata map[string]string) DefinitionOption {
	return func(definition *Definition) {
		definition.Metadata = metadata
	}
}

func (d *Definition) Mapping() map[string]*Definition {
	mapping := make(map[string]*Definition)
	mapping["root"] = d

	for name, child := range d.children {
		child.mapChild("root", name, mapping)
	}

	return mapping
}

func (d *Definition) mapChild(prefix string, name string, mapping map[string]*Definition) {
	key := prefix + "." + name
	mapping[key] = d

	for name, child := range d.children {
		child.mapChild(key, name, mapping)
	}
}

type BuildInfo struct {
	Key             string
	ReplacementID   string
	DependentBuilds []BuildInfo
}

func (d *Definition) BuildInfo() BuildInfo {
	buildInfo := BuildInfo{Key: "root"}

	for name, child := range d.children {
		buildInfo.DependentBuilds = append(buildInfo.DependentBuilds, child.childBuildInfo("root", name))
	}

	return buildInfo
}

func (d *Definition) childBuildInfo(prefix string, name string) BuildInfo {
	key := prefix + "." + name
	buildInfo := BuildInfo{Key: key, ReplacementID: name}

	for name, child := range d.children {
		buildInfo.DependentBuilds = append(buildInfo.DependentBuilds, child.childBuildInfo(key, name))
	}

	return buildInfo
}

func (d *Definition) DynamicParts() []string {
	return d.dynamicParts
}

func (d *Definition) UrlWithParams(parameters url.Values) *url.URL {
	// This is already parsed before constructing the url in server.go, so we ignore errors
	targetUrl, _ := url.Parse(d.Url)
	targetUrl.RawQuery = parameters.Encode()

	return targetUrl
}

func (d *Definition) Requestable(target *url.URL, pathParams map[string]string, query url.Values) (*Request, error) {
	request := *target // clone the url

	var path strings.Builder

	for _, part := range d.routeParts {
		path.WriteByte('/')

		if strings.HasPrefix(part, ":") {
			if replacement, ok := pathParams[part]; ok {
				path.WriteString(replacement)
			} else {
				return nil, fmt.Errorf("no parameter was provided for %s in route %s", part, d.Path)
			}
		} else {
			path.WriteString(part)
		}
	}

	unescapedPath, err := url.PathUnescape(path.String())
	if err != nil {
		return nil, fmt.Errorf("could not encode url: %w", err)
	}
	request.Path = unescapedPath    // Set unescaped path which treats %2f as a /
	request.RawPath = path.String() // Set RawPath which lets go correlate %2f to / in the Path, and escape correctly when calling String()

	request.RawQuery = query.Encode()

	return &Request{
		RequestURL: &request,
		Definition: d,
	}, nil
}

type Request struct {
	RequestURL *url.URL
	Definition *Definition
	name       string
}

var _ multiplexer.Requestable = &Request{}

func (fr *Request) Name() string                { return fr.name }
func (fr *Request) URL() string                 { return fr.RequestURL.String() }
func (fr *Request) Metadata() map[string]string { return fr.Definition.Metadata }

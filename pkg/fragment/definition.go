package fragment

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/blakewilliams/viewproxy/pkg/multiplexer"
)

type Children = map[string]*Definition
type Collection = []*Definition
type DefinitionOption = func(*Definition)

type Definition struct {
	Path             string
	routeParts       []string
	dynamicParts     []string
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

func (d *Definition) Children() map[string]*Definition {
	return d.children
}

func (d *Definition) Child(name string) *Definition {
	return d.children[name]
}

func WithChildren(children Children) DefinitionOption {
	return func(definition *Definition) {
		for name, child := range children {
			definition.children[name] = child
		}
	}
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

func (d *Definition) DynamicParts() []string {
	return d.dynamicParts
}

func (d *Definition) Requestable(target *url.URL, pathParams map[string]string, query url.Values) (*Request, error) {
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

	request, err := buildURL(target, path.String(), query.Encode())
	if err != nil {
		return nil, err
	}

	templateURL, err := buildURL(target, strings.Join(d.routeParts, "/"), "")
	if err != nil {
		return nil, err
	}

	return &Request{
		RequestURL:  request,
		Definition:  d,
		templateURL: templateURL,
	}, nil
}

func buildURL(base *url.URL, path string, query string) (*url.URL, error) {
	unescapedPath, err := url.PathUnescape(path)
	if err != nil {
		return nil, fmt.Errorf("could not encode url: %w", err)
	}

	u := *base // clone the url
	u.RawQuery = query
	u.Path = unescapedPath // Set unescaped path which treats %2f as a /
	u.RawPath = path       // Set RawPath which lets go correlate %2f to / in the Path, and escape correctly when calling String()

	return &u, nil
}

type Request struct {
	RequestURL  *url.URL
	Definition  *Definition
	templateURL *url.URL
}

var _ multiplexer.Requestable = &Request{}

func (fr *Request) URL() string                 { return fr.RequestURL.String() }
func (fr *Request) TemplateURL() string         { return fr.templateURL.String() }
func (fr *Request) Metadata() map[string]string { return fr.Definition.Metadata }

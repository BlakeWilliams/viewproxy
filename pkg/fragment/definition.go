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
	TimingLabel      string
	IgnoreValidation bool `json:"ignoreValidation"`
}

func Define(path string, options ...DefinitionOption) *Definition {
	safePath := strings.TrimPrefix(path, "/")
	definition := &Definition{
		Path:       path,
		routeParts: strings.Split(safePath, "/"),
		Metadata:   make(map[string]string),
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

func WithTimingLabel(timingLabel string) DefinitionOption {
	return func(definition *Definition) {
		definition.TimingLabel = timingLabel
	}
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

func (d *Definition) Requestable(target *url.URL, parameters map[string]string, query url.Values) (*Request, error) {
	request := *target // clone the url

	var path strings.Builder

	for _, part := range d.routeParts {
		path.WriteByte('/')

		if strings.HasPrefix(part, ":") {
			if replacement, ok := parameters[part]; ok {
				path.WriteString(replacement)
			} else {
				return nil, fmt.Errorf("no url replacement found for %s in %s", part, d.Path)
			}
		} else {
			path.WriteString(part)
		}
	}

	request.Path = path.String()
	request.RawQuery = query.Encode()

	return &Request{
		RequestURL: &request,
		Definition: d,
	}, nil
}

func (d *Definition) PreloadUrl(target string) {
	targetUrl, err := url.Parse(
		fmt.Sprintf("%s/%s", strings.TrimRight(target, "/"), strings.TrimLeft(d.Path, "/")),
	)

	if err != nil {
		// It should be okay to panic here, since this should only be called at boot time
		panic(err)
	}

	d.Url = targetUrl.String()
}

type Request struct {
	RequestURL *url.URL
	Definition *Definition
}

var _ multiplexer.Requestable = &Request{}

func (fr *Request) URL() string                 { return fr.RequestURL.String() }
func (fr *Request) Metadata() map[string]string { return fr.Definition.Metadata }
func (fr *Request) TimingLabel() string         { return fr.Definition.TimingLabel }

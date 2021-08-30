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
	Path        string `json:"path"`
	Url         string
	Metadata    map[string]string `json:"metadata"`
	TimingLabel string            `json:"timingLabel"`
}

func Define(path string, options ...DefinitionOption) *Definition {
	definition := &Definition{
		Path:     path,
		Metadata: make(map[string]string),
	}

	for _, option := range options {
		option(definition)
	}

	return definition
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

func (d *Definition) UrlWithParams(parameters url.Values) string {
	// This is already parsed before constructing the url in server.go, so we ignore errors
	targetUrl, _ := url.Parse(d.Url)
	targetUrl.RawQuery = parameters.Encode()

	return targetUrl.String()
}

func (d *Definition) IntoRequestable(params url.Values) multiplexer.Requestable {
	return &Request{
		url:        d.UrlWithParams(params),
		Definition: d,
	}
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
	url        string
	Definition *Definition
}

var _ multiplexer.Requestable = &Request{}

func (fr *Request) URL() string                 { return fr.url }
func (fr *Request) Metadata() map[string]string { return fr.Definition.Metadata }
func (fr *Request) TimingLabel() string         { return fr.Definition.TimingLabel }

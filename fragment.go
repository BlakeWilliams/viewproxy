package viewproxy

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/blakewilliams/viewproxy/pkg/multiplexer"
)

type FragmentRoute struct {
	Path        string `json:"path"`
	Url         string
	Metadata    map[string]string `json:"metadata"`
	TimingLabel string            `json:"timingLabel"`
}

func NewFragment(path string) *FragmentRoute {
	return &FragmentRoute{
		Path:     path,
		Metadata: make(map[string]string),
	}
}

func NewFragmentWithMetadata(path string, metadata map[string]string) *FragmentRoute {
	return &FragmentRoute{
		Path:     path,
		Metadata: metadata,
	}
}

func (f *FragmentRoute) UrlWithParams(parameters url.Values) string {
	// This is already parsed before constructing the url in server.go, so we ignore errors
	targetUrl, _ := url.Parse(f.Url)
	targetUrl.RawQuery = parameters.Encode()

	return targetUrl.String()
}

func (f *FragmentRoute) IntoRequestable(params url.Values) multiplexer.Requestable {
	return &fragmentRequest{
		url:           f.UrlWithParams(params),
		fragmentRoute: f,
	}
}

func (f *FragmentRoute) PreloadUrl(target string) {
	targetUrl, err := url.Parse(
		fmt.Sprintf("%s/%s", strings.TrimRight(target, "/"), strings.TrimLeft(f.Path, "/")),
	)

	if err != nil {
		// It should be okay to panic here, since this should only be called at boot time
		panic(err)
	}

	f.Url = targetUrl.String()
}

type fragmentRequest struct {
	url           string
	fragmentRoute *FragmentRoute
}

var _ multiplexer.Requestable = &fragmentRequest{}

func (fr *fragmentRequest) URL() string                 { return fr.url }
func (fr *fragmentRequest) Metadata() map[string]string { return fr.fragmentRoute.Metadata }
func (fr *fragmentRequest) TimingLabel() string         { return fr.fragmentRoute.TimingLabel }

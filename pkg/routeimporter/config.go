package routeimporter

import (
	"github.com/blakewilliams/viewproxy"
	"github.com/blakewilliams/viewproxy/pkg/fragment"
)

type ConfigRouteEntry struct {
	Url       string               `json:"url"`
	Layout    *fragment.Definition `json:"layout"`
	Fragments fragment.Collection  `json:"fragments"`
	Metadata  map[string]string    `json:"metadata"`
}

func LoadRoutes(server *viewproxy.Server, routeEntries []ConfigRouteEntry) error {
	for _, routeEntry := range routeEntries {
		server.Get(routeEntry.Url, routeEntry.Layout, routeEntry.Fragments, viewproxy.WithRouteMetadata(routeEntry.Metadata))
	}

	return nil
}

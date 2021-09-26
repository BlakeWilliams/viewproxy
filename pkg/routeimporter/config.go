package routeimporter

import (
	"github.com/blakewilliams/viewproxy"
	"github.com/blakewilliams/viewproxy/pkg/fragment"
)

type ConfigRouteEntry struct {
	Url           string               `json:"url"`
	LayoutData    *fragment.Definition `json:"layout"`
	FragmentsData fragment.Collection  `json:"fragments"`
	Metadata      map[string]string    `json:"metadata"`
}

func LoadRoutes(server *viewproxy.Server, routeEntries []ConfigRouteEntry) error {
	for _, routeEntry := range routeEntries {
		layout := fragment.Define(routeEntry.LayoutData.Path, fragment.WithMetadata(routeEntry.LayoutData.Metadata))
		layout.TimingLabel = routeEntry.LayoutData.TimingLabel
		fragments := make(fragment.Collection, len(routeEntry.FragmentsData))

		for i, fragmentData := range routeEntry.FragmentsData {
			fragments[i] = fragment.Define(fragmentData.Path, fragment.WithMetadata(fragmentData.Metadata))
			fragments[i].TimingLabel = fragmentData.TimingLabel
		}

		server.Get(
			routeEntry.Url,
			layout,
			fragments,
			viewproxy.WithRouteMetadata(routeEntry.Metadata),
		)
	}

	return nil
}

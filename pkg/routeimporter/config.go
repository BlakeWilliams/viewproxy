package routeimporter

import (
	"github.com/blakewilliams/viewproxy"
	"github.com/blakewilliams/viewproxy/pkg/fragment"
)

type ConfigRouteEntry struct {
	Url               string               `json:"url"`
	LayoutTemplate    *fragment.Definition `json:"layout"`
	FragmentsTemplate fragment.Collection  `json:"fragments"`
	Metadata          map[string]string    `json:"metadata"`
	IgnoreValidation  bool                 `json:"ignoreValidation"`
}

func LoadRoutes(server *viewproxy.Server, routeEntries []ConfigRouteEntry) error {
	for _, routeEntry := range routeEntries {
		layout := createFragment(routeEntry.LayoutTemplate)

		fragments := make(fragment.Collection, len(routeEntry.FragmentsTemplate))
		for i, fragmentTemplate := range routeEntry.FragmentsTemplate {
			fragments[i] = createFragment(fragmentTemplate)
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

func createFragment(template *fragment.Definition) *fragment.Definition {
	fragment := fragment.Define(template.Path, fragment.WithMetadata(template.Metadata))
	fragment.TimingLabel = template.TimingLabel

	if template.IgnoreValidation {
		fragment.IgnoreValidation = true
	}

	return fragment
}

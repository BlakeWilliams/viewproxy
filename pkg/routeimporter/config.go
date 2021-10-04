package routeimporter

import (
	"github.com/blakewilliams/viewproxy"
	"github.com/blakewilliams/viewproxy/pkg/fragment"
)

type ConfigRouteEntry struct {
	Url               string               `json:"url"`
	LayoutTemplate    *fragment.Definition `json:"layout"`
	FragmentTemplates fragment.Collection  `json:"fragments"`
	Metadata          map[string]string    `json:"metadata"`
	IgnoreValidation  bool
}

func LoadRoutes(server *viewproxy.Server, routeEntries []ConfigRouteEntry) error {
	for _, routeEntry := range routeEntries {
		layout := createFragment(routeEntry.LayoutTemplate)

		fragments := make(fragment.Collection, len(routeEntry.FragmentTemplates))
		for i, fragmentTemplate := range routeEntry.FragmentTemplates {
			fragments[i] = createFragment(fragmentTemplate)
		}

		err := server.Get(
			routeEntry.Url,
			layout,
			fragments,
			viewproxy.WithRouteMetadata(routeEntry.Metadata),
		)

		if err != nil {
			return err
		}
	}

	return nil
}

func createFragment(template *fragment.Definition) *fragment.Definition {
	fragment := fragment.Define(template.Path, fragment.WithMetadata(template.Metadata))
	fragment.IgnoreValidation = template.IgnoreValidation

	return fragment
}

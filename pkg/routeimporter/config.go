package routeimporter

import (
	"github.com/blakewilliams/viewproxy"
	"github.com/blakewilliams/viewproxy/pkg/fragment"
)

type ConfigFragment struct {
	Path             string
	Metadata         map[string]string
	IgnoreValidation bool
	Children         map[string]ConfigFragment
}

type ConfigRouteEntry struct {
	Path              string            `json:"url"`
	Root             ConfigFragment    `json:"root"`
	Metadata         map[string]string `json:"metadata"`
	IgnoreValidation bool
}

func LoadRoutes(server *viewproxy.Server, routeEntries []ConfigRouteEntry) error {
	for _, routeEntry := range routeEntries {
		root := createFragment(routeEntry.Root)

		err := server.Get(
			routeEntry.Url,
			root,
			viewproxy.WithRouteMetadata(routeEntry.Metadata),
		)

		if err != nil {
			return err
		}
	}

	return nil
}

func createFragment(template ConfigFragment) *fragment.Definition {
	f := fragment.Define(template.Path, fragment.WithMetadata(template.Metadata))
	f.IgnoreValidation = template.IgnoreValidation

	for name, child := range template.Children {
		fragment.WithChild(name, createFragment(child))(f)
	}

	return f
}

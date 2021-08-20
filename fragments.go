package viewproxy

import "github.com/blakewilliams/viewproxy/pkg/fragments"

type ContentFragments = []*fragments.Definition
type DefinitionOption = fragments.DefinitionOption

// DefineFragment creates a new fragments.Definition targetting the given path.
func DefineFragment(path string, opts ...DefinitionOption) *fragments.Definition {
	return fragments.New(path, opts...)
}

func WithFragmentMetadata(metadata map[string]string) DefinitionOption {
	return fragments.WithMetadata(metadata)
}

// DefineFragments creates a new []*fragments.Definition for each string in
// paths. This can be used in combination with the Server.Get function to define
// routes.
func DefineFragments(paths ...string) ContentFragments {
	definitions := make(ContentFragments, len(paths))

	for i, path := range paths {
		definitions[i] = DefineFragment(path)
	}

	return definitions
}

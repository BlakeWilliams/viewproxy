package viewproxy

import "github.com/blakewilliams/viewproxy/pkg/fragment"

type fragmentStitchStructure struct {
	Key                 string
	ReplacementID       string
	DependentStructures []fragmentStitchStructure
}

func stitchStructureFor(d *fragment.Definition) fragmentStitchStructure {
	structure := fragmentStitchStructure{Key: "root"}

	for name, child := range d.Children() {
		structure.DependentStructures = append(structure.DependentStructures, childStitchStructure("root", name, child))
	}

	return structure
}

func childStitchStructure(prefix string, name string, d *fragment.Definition) fragmentStitchStructure {
	key := prefix + "." + name
	buildInfo := fragmentStitchStructure{Key: key, ReplacementID: name}

	for name, child := range d.Children() {
		buildInfo.DependentStructures = append(buildInfo.DependentStructures, childStitchStructure(key, name, child))
	}

	return buildInfo
}

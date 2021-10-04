package viewproxy

import "github.com/blakewilliams/viewproxy/pkg/fragment"

type stitchStructure struct {
	key                 string
	replacementID       string
	dependentStructures []*stitchStructure
}

func (s *stitchStructure) Key() string {
	return s.key
}

func (s *stitchStructure) ReplacementID() string {
	return s.replacementID
}

func (s *stitchStructure) DependentStructures() []*stitchStructure {
	return s.dependentStructures
}

func stitchStructureFor(d *fragment.Definition) *stitchStructure {
	structure := &stitchStructure{key: "root"}

	for name, child := range d.Children() {
		structure.dependentStructures = append(structure.dependentStructures, childStitchStructure("root", name, child))
	}

	return structure
}

func childStitchStructure(prefix string, name string, d *fragment.Definition) *stitchStructure {
	key := prefix + "." + name
	buildInfo := &stitchStructure{key: key, replacementID: name}

	for name, child := range d.Children() {
		buildInfo.dependentStructures = append(buildInfo.dependentStructures, childStitchStructure(key, name, child))
	}

	return buildInfo
}

package viewproxy

import (
	"testing"

	"github.com/blakewilliams/viewproxy/pkg/fragment"
	"github.com/stretchr/testify/require"
)

func TestStitchStructure(t *testing.T) {
	rootFragment := fragment.Define("layout", fragment.WithChildren(fragment.Children{
		"header": fragment.Define("header"),
		"body": fragment.Define("body", fragment.WithChildren(fragment.Children{
			"main":    fragment.Define("main"),
			"sidebar": fragment.Define("sidebar"),
		})),
	}))

	structure := stitchStructureFor(rootFragment)

	var headerStructure *stitchStructure
	var bodyStructure *stitchStructure

	// Maps are not ordered, so we need to find the correct fragment order here
	if structure.DependentStructures()[0].Key() == "root.header" {
		headerStructure = structure.DependentStructures()[0]
		bodyStructure = structure.DependentStructures()[1]
	} else {
		headerStructure = structure.DependentStructures()[1]
		bodyStructure = structure.DependentStructures()[0]
	}

	require.Equal(t, "root", structure.Key())

	require.Equal(t, "root.header", headerStructure.Key())
	require.Equal(t, "header", headerStructure.ReplacementID())

	require.Equal(t, "root.body", bodyStructure.Key())
	require.Equal(t, "body", bodyStructure.ReplacementID())

	var bodyKeys []string
	var bodyReplacementIDs []string
	for _, structure := range bodyStructure.DependentStructures() {
		bodyKeys = append(bodyKeys, structure.Key())
		bodyReplacementIDs = append(bodyReplacementIDs, structure.ReplacementID())
	}

	require.Contains(t, bodyKeys, "root.body.main")
	require.Contains(t, bodyKeys, "root.body.sidebar")

	require.Contains(t, bodyReplacementIDs, "main")
	require.Contains(t, bodyReplacementIDs, "sidebar")
}

package viewproxy

import (
	"encoding/json"
	"io/ioutil"
	"os"

	"github.com/blakewilliams/viewproxy/pkg/fragments"
)

type configRouteEntry struct {
	Url       string                `json:"url"`
	Layout    *fragments.Definition `json:"layout"`
	Fragments ContentFragments      `json:"fragments"`
	Metadata  map[string]string     `json:"metadata"`
}

func readConfigFile(filePath string) ([]configRouteEntry, error) {
	file, err := os.Open(filePath)

	if err != nil {
		return nil, err
	}

	routesJson, err := ioutil.ReadAll(file)

	if err != nil {
		return nil, err
	}

	return loadJsonConfig(routesJson)
}

func loadJsonConfig(routesJson []byte) ([]configRouteEntry, error) {
	var routeEntries []configRouteEntry

	if err := json.Unmarshal(routesJson, &routeEntries); err != nil {
		return nil, err
	}

	return routeEntries, nil
}

package viewproxy

import (
	"encoding/json"
	"io/ioutil"
	"os"
)

type configRouteEntry struct {
	Url               string   `json:"url"`
	LayoutFragmentUrl string   `json:"layout"`
	FragmentUrls      []string `json:"fragments"`
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

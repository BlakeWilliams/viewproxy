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

	var routeEntries []configRouteEntry

	err = json.Unmarshal(routesJson, &routeEntries)

	if err != nil {
		return nil, err
	}

	return routeEntries, nil
}

package viewproxy

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/blakewilliams/viewproxy/pkg/fragment"
)

type configRouteEntry struct {
	Url       string               `json:"url"`
	Layout    *fragment.Definition `json:"layout"`
	Fragments fragment.Collection  `json:"fragments"`
	Metadata  map[string]string    `json:"metadata"`
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

func loadHttpConfigFile(target string) ([]configRouteEntry, error) {
	var routeEntries []configRouteEntry

	resp, err := http.Get(target)

	if err != nil {
		return nil, fmt.Errorf("could not fetch JSON configuration: %w", err)
	}

	routesJson, err := io.ReadAll(resp.Body)

	if err != nil {
		return nil, fmt.Errorf("could not read route config response body: %w", err)
	}

	if err := json.Unmarshal(routesJson, &routeEntries); err != nil {
		return routeEntries, fmt.Errorf("could not unmarshal route config json: %w", err)
	}

	return routeEntries, nil
}

package routeimporter

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/blakewilliams/viewproxy"
)

func LoadJSONFile(server *viewproxy.Server, filepath string) error {
	file, err := os.Open(filepath)

	if err != nil {
		return fmt.Errorf("could not open config file: %w", err)
	}

	routesJSON, err := ioutil.ReadAll(file)

	if err != nil {
		return fmt.Errorf("could not read config file: %w", err)
	}

	err = LoadJSON(server, []byte(routesJSON))

	if err != nil {
		return fmt.Errorf("could not load config: %w", err)
	}

	return nil
}

func LoadJSON(server *viewproxy.Server, routesJSON []byte) error {
	var routeEntries []ConfigRouteEntry

	if err := json.Unmarshal(routesJSON, &routeEntries); err != nil {
		return fmt.Errorf("could not unmarshal in loadJSON: %w", err)
	}

	err := LoadRoutes(server, routeEntries)

	if err != nil {
		return fmt.Errorf("could not unmarshal in loadJSON: %w", err)
	}

	return nil
}

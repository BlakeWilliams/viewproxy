package routeimporter

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/blakewilliams/viewproxy"
)

func LoadHttp(server *viewproxy.Server, path string) error {
	var routeEntries []ConfigRouteEntry

	target, err := url.Parse(server.Target())

	if err != nil {
		return fmt.Errorf("could not parse target: %w", err)
	}

	target.Path = path
	req, err := http.NewRequest(http.MethodGet, target.String(), nil)

	if err != nil {
		return fmt.Errorf("Could not create a request when loading config: %w", err)
	}

	if server.HmacSecret != "" {
		SetHmacHeaders(req, server.HmacSecret)
	}

	resp, err := http.DefaultClient.Do(req)

	if err != nil {
		return fmt.Errorf("could not fetch JSON configuration: %w", err)
	}

	routesJson, err := io.ReadAll(resp.Body)

	if err != nil {
		return fmt.Errorf("could not read route config response body: %w", err)
	}

	if err := json.Unmarshal(routesJson, &routeEntries); err != nil {
		return fmt.Errorf("could not unmarshal route config json: %w", err)
	}

	err = LoadRoutes(server, routeEntries)

	if err != nil {
		return fmt.Errorf("could not load routes into server: %w", err)
	}

	return nil
}

func SetHmacHeaders(r *http.Request, hmacSecret string) {
	timestamp := fmt.Sprintf("%d", time.Now().Unix())

	mac := hmac.New(sha256.New, []byte(hmacSecret))
	mac.Write(
		[]byte(fmt.Sprintf("%s,%s", r.URL.Path, timestamp)),
	)

	r.Header.Set("Authorization", hex.EncodeToString(mac.Sum(nil)))
	r.Header.Set("X-Authorization-Time", timestamp)
}

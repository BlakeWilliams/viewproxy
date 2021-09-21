package routeimporter

import (
	"io/ioutil"
	"log"
	"os"
	"testing"

	"github.com/blakewilliams/viewproxy"
)

func TestLoadJSONFile(t *testing.T) {
	viewproxyServer := viewproxy.NewServer("http://fake.net")
	viewproxyServer.Logger = log.New(ioutil.Discard, "", log.Ldate|log.Ltime)

	// Load routes from config
	file, err := ioutil.TempFile(os.TempDir(), "config.json")
	if err != nil {
		t.Error(err)
	}
	defer os.Remove(file.Name())

	file.Write([]byte(jsonConfig))
	file.Close()

	LoadJSONFile(viewproxyServer, file.Name())

	requireJsonConfigRoutesLoaded(t, viewproxyServer.Routes())
}

package routeimporter

import (
	"io/ioutil"
	"log"
	"os"
	"testing"

	"github.com/blakewilliams/viewproxy"
	"github.com/stretchr/testify/require"
)

func TestLoadJSONFile(t *testing.T) {
	viewproxyServer, err := viewproxy.NewServer("http://fake.net")
	require.NoError(t, err)
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

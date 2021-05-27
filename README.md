# View-Proxy

View-Proxy is a Go service that makes multiple requests to an application in parallel, fetching HTML content and stitching it together to serve to a user.

This is alpha software, and is currently a proof of concept used in conjunction with Rails and View Component as a performance optimization.

## Usage

See `cmd/demo/main.go` for an example of how to use the package.

To use view-proxy:

```go
import "github.com/blakewilliams/view-proxy/internal/server"
	timeout, err := time.ParseDuration("5s")

// Create a new Server Instance
	server := &server.Server{
		Port:         3005,
		ProxyTimeout: timeout,
		// View-Proxy will hit this URL, forwarding named URL parameters as query params.
		// The `fragment` query param is the name of the requested fragment to render.
		Target:       "http://localhost:3000/_view_fragments",
		Logger:       log.Default,
	}

	// Define a route with a :name parameter that will be forwarded to the target host.
	// This will make 3 fragment requests, one for the header, hello, and footer.
	server.Get("/hello/:name", []string{
		"header", // GET http://localhost:3000/_view_fragments?fragment=header&name=world
		"hello",  // GET http://localhost:3000/_view_fragments?fragment=hello&name=world
		"footer", // GET http://localhost:3000/_view_fragments?fragment=footer&name=world
	})

	server.ListenAndServe()
```

## Demo Usage

* The port the server is bound to can be set via the `PORT` environment variable.
* The target server can be set via the `TARGET` environment variable.
  * The default is `localhost:3000/_view_fragments`
  * View-Proxy will call that end-point with the fragment name being passed as a query parameter. e.g.  `localhost:3000/_view_fragments?fragment=header`

To run view-proxy, run `go build ./cmd/demo && ./demo`

## To-Do

* [x] Add logging
* [ ] Come up with a solution for query param forwarding
* [x] Add tests for the core workflows
* [x] Follow a better application structure (`cmd` directory, `pkg` directory, etc)

# viewproxy

`viewproxy` is a Go service that makes multiple requests to an application in parallel, fetching HTML content and stitching it together to serve to a user.

This is alpha software, and is currently a proof of concept used in conjunction with Rails and View Component as a performance optimization.

## Usage

See `cmd/demo/main.go` for an example of how to use the package.

To use `viewproxy`:

```go
import "github.com/blakewilliams/viewproxy"

// Create and configure a new Server Instance
server := viewproxy.NewServer(target)
server.Port = 3005
server.ProxyTimeout = time.Duration(5) * time.Second
server.DefaultPageTitle = "Demo app"
server.IgnoreHeader("etag")
server.PassThrough = true

// Define a route with a :name parameter that will be forwarded to the target host.
// This will make a layout request and 3 fragment requests, one for the header, hello, and footer.

// GET http://localhost:3000/_view_fragments/layouts/my_layout?name=world
layout := viewproxy.NewFragment("my_layout")
server.Get("/hello/:name", layout, []*viewproxy.Fragment{
	viewproxy.NewFragment("header"), // GET http://localhost:3000/_view_fragments/header?name=world
	viewproxy.NewFragment("hello"),  // GET http://localhost:3000/_view_fragments/hello?name=world
	viewproxy.NewFragment("footer"), // GET http://localhost:3000/_view_fragments/footer?name=world
})

server.ListenAndServe()
```

## Demo Usage

- The port the server is bound to `3005` by default but can be set via the `PORT` environment variable.
- The target server can be set via the `TARGET` environment variable.
  - The default is `localhost:3000/_view_fragments`
  - `viewproxy` will call that end-point with the fragment name being passed as a query parameter. e.g. `localhost:3000/_view_fragments?fragment=header`

To run `viewproxy`, run `go build ./cmd/demo && ./demo`

## Tracing with Open Telemetry

You can use tracing to learn which fragment(s) are slowest for a given page, so you know where to optimize.

To set up distributed tracing via [Open Telemetry](https://opentelemetry.io):

```go
viewProxyServer.ConfigureTracing(
	"localhost:8360", // endpoint
	"my-viewproxy-service", // service name
	false, // insecure mode?
}
```

### Tracing attributes via fragment metadata

Each fragment can be configured with a static map of key/values, which will be set as tracing attributes when each fragment is fetched.

```go
layout := viewproxy.NewFragment("my_layout")
server.Get("/hello/:name", layout, []*viewproxy.Fragment{
	viewproxy.NewFragmentWithMetadata("header", map[string]string{"page": "homepage"}), // spans will have a "page" attribute with value "homepage"
})
```

## Philosophy

`viewproxy` is a simple service designed to sit between a browser request and a web application. It is used to break pages down into fragments that can be rendered in parallel for faster response times.

- `viewproxy` is not coupled to a specific application framework, but _is_ being driven by close integration with Rails applications.
- `viewproxy` should rely on Rails' (or other target application framework) strengths when possible.
- `viewproxy` itself and its client API's should focus on developer happiness and productivity.

## Development

Run the tests:

```
go test ./...
```

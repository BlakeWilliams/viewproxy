# View-Proxy

View-Proxy is a Go service that makes multiple requests to an application in parallel, fetching HTML content and stitching it together to serve to a user.

This is alpha software, and is currently a proof of concept used in conjunction with Rails and View Component as a performance optimization.

## Usage

* The port the server is bound to can be set via the `PORT` environment variable.
* The target server can be set via the `TARGET` environment variable.
  * The default is `localhost:3000/_view_fragments`
  * View-Proxy will call that end-point with the fragment name being passed as a query parameter. e.g.  `localhost:3000/_view_fragments?fragment=header`

To run view-proxy, run `go run .`

## To-Do

* [ ] Add logging
* [ ] Come up with a solution for query param forwarding
* [ ] Add tests for the core workflows
* [ ] Follow a better application structure (`cmd` directory, `pkg` directory, etc)

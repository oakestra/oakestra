# client

A Go HTTP client for [`go_resource_abstractor`](..) (and its Python predecessor,
[`resource-abstractor`](../../resource-abstractor) - they serve the same wire-compatible
`/api/v1` REST API). It plays the same role for Go services that
[`resource_abstractor_client`](../../libraries/resource_abstractor_client) plays for Python
services: a thin wrapper around the applications, resources (candidates), and jobs endpoints, so
consumers don't hand-roll HTTP calls against the abstractor.

## Why a separate module

This is its own Go module (`go.mod` in this directory), nested inside `go_resource_abstractor/`
purely for co-location with the server it talks to - it imports nothing from the server and the
server imports nothing from it. Go automatically treats a subdirectory with its own `go.mod` as
excluded from the parent module, so:

- The server's build is completely unaffected (`go build .` / `go build ./...` from
  `go_resource_abstractor/` never descends into `client/`).
- The client's dependency graph stays minimal - **stdlib only** (`net/http`,
  `encoding/json`, ...), in keeping with Oakestra's lightweight-first design principle. No gin,
  mongo-driver, or testcontainers leak into a consumer's build.

This mirrors why the client shares no types with the server: the server itself is
schema-agnostic (every endpoint accepts/returns plain JSON documents, validated against a spec
rather than bound to a Go struct - see `go_resource_abstractor/api/*_schema.go`), so there's
nothing to reuse. The client's `Document` type (`map[string]any`) matches that shape directly.

## Usage

```go
import (
    "context"
    "errors"
    "log"

    "github.com/oakestra/oakestra/go_resource_abstractor/client"
)

func main() {
    c, err := client.NewFromEnv() // reads RESOURCE_ABSTRACTOR_URL / RESOURCE_ABSTRACTOR_PORT
    if err != nil {
        log.Fatal(err)
    }

    ctx := context.Background()

    candidates, err := c.Resources.List(ctx, map[string]string{"active": "true"})
    if err != nil {
        log.Fatal(err)
    }

    job, err := c.Jobs.GetByID(ctx, "507f1f77bcf86cd799439011")
    switch {
    case errors.Is(err, client.ErrNotFound):
        // no such job
    case err != nil:
        // transport error, or a non-2xx/404 response (*client.APIError)
    default:
        // use job
    }
}
```

`client.New(baseURL, opts...)` builds a client against an explicit base URL (no `/api/v1` suffix,
no trailing slash) when you don't want to read it from the environment. `WithHTTPClient` and
`WithTimeout` (default 10s) are available as `Option`s.

### Error handling

Every method returns `(value, error)`. This is a deliberate improvement over the Python client,
which returns `None` for a 404, a connection failure, *and* any other non-2xx response alike, so
callers can't distinguish "not found" from "the request failed":

- `errors.Is(err, client.ErrNotFound)` - the abstractor returned 404. Used by `GetByID` calls and
  by the `GetByName`/`GetByIP`/`GetByNameAndNamespace` helpers when the underlying list came back
  empty (mirroring the Python client's `result[0] if result else None` pattern).
- `errors.As(err, &apiErr)` where `apiErr` is `*client.APIError` - any other non-2xx response.
  Carries `Status` and, when the abstractor's `{"message": "..."}` error body could be decoded,
  `Message`.
- Any other `error` - a transport-level failure (DNS, connection refused, context
  cancellation/deadline).

## API surface

Matches the endpoints the Python `resource_abstractor_client` actually has consumers for:
applications, resources (candidates), and jobs (including the instance sub-routes). See
`applications.go`, `resources.go`, and `jobs.go` for the full method list and their Python
equivalents (documented per-method). Hooks and custom-resources are intentionally not covered -
no current consumer needs them; add a `hooks.go` / `customresources.go` the same way if that
changes.

## Consuming this from another Go service

Each Go binary in this repo (`scheduler`, `go_node_engine`, `go_resource_abstractor` itself)
builds in its own Docker context and none of them currently import another local Go module - see
the top-level module names (`scheduler`, `go_node_engine`, `go_resource_abstractor`, all bare, not
`github.com/...` paths). Adopting this client from one of them means:

1. In the consumer's `go.mod`, add:
   ```
   require github.com/oakestra/oakestra/go_resource_abstractor/client v0.0.0
   replace github.com/oakestra/oakestra/go_resource_abstractor/client => ../go_resource_abstractor/client
   ```
2. Widen that service's Docker build context to include `../go_resource_abstractor/client`
   (currently each service's Dockerfile only `COPY`s its own directory) and add the corresponding
   `COPY`/`go mod download` lines for the client's `go.mod`.

No consumer is migrated as part of adding this library; this is the recipe for when one is.

## Testing

```bash
go build ./...
go vet ./...
go test ./...
```

Tests stub the abstractor with `net/http/httptest` - no MongoDB or Docker required, unlike the
server's own test suite.

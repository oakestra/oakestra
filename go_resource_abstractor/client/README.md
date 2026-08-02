# client

A Go HTTP client for [`go_resource_abstractor`](..) (and its Python predecessor,
[`resource-abstractor`](../../resource-abstractor), which serve the same `/api/v1` REST API). It
plays the same role for Go services that
[`resource_abstractor_client`](../../libraries/resource_abstractor_client) plays for Python
services: a thin wrapper around the applications, resources (candidates), jobs and hooks
endpoints, so consumers don't hand-roll HTTP calls against the abstractor.

## Generated from the service's own spec

The paths, query parameter encoding, request bodies and models all come from
[`../openapi/openapi.yaml`](../openapi/openapi.yaml) - the same single file the server's routes
and `ServerInterface` are generated from, one directory up and one module over.
[oapi-codegen](https://github.com/oapi-codegen/oapi-codegen) reads it twice, in opposite
directions:

```
                       openapi/openapi.yaml
                        /                \
       gin ServerInterface              ClientInterface
      openapi/openapi.gen.go       client/openapi/openapi.gen.go
```

That keeps the two halves of the contract from disagreeing: a spec change that renames a query
parameter or moves a route changes both sides at once, and CI fails if either generated file
wasn't regenerated.

This package is the ergonomic layer over that generated client: construction from the
environment, the `ErrNotFound`/`APIError` split, filter options instead of pointer-filled param
structs, `Decode`/`Done` to carry that same split over calls made straight against the generated
client, and the handful of lookups the Python client has consumers for. **Consumers import
`client` only** - every document type is re-exported here (`client.Job`, `client.Resource`,
`client.Hook`, ...), and ids and request bodies are aliases of those same document types, so there
is no path through this package's surface, including `Client.OpenAPI()`, that ever requires
calling code to import `openapi`. `Client.OpenAPI()` reaches the generated client directly for
anything the facade doesn't wrap.

Those re-exports are *type aliases*, not new types - `client.Job` and `openapi.Job` are the same
type. Values pass freely between the facade and `Client.OpenAPI()`, and a field added to the spec
shows up here the moment the code is regenerated, with nothing to keep in step by hand.

### Regenerating

```bash
go generate ./openapi     # after editing ../openapi/openapi.yaml
```

`openapi/openapi.gen.go` is committed, so building a consumer never runs a code generator. A
pre-commit hook regenerates both halves whenever the spec changes; CI fails if they drift. The
generator version is pinned in the `go:generate` directive in `openapi/gen.go` and must match the
one the server pins in `../openapi/spec.go`.

## Why a separate module

This is its own Go module (`go.mod` in this directory), nested inside `go_resource_abstractor/`
purely for co-location with the server it talks to - it imports nothing from the server and the
server imports nothing from it. Go treats a subdirectory with its own `go.mod` as excluded from
the parent module, so:

- The server's build is unaffected (`go build .` / `go build ./...` from
  `go_resource_abstractor/` never descends into `client/`).
- The client's dependency graph stays minimal - stdlib plus
  [`oapi-codegen/runtime`](https://github.com/oapi-codegen/runtime), the small pure-Go package the
  generated request builders use to encode parameters (it brings `go-jsonmerge` and `google/uuid`
  with it, nothing else). No gin, mongo-driver or testcontainers leaks into a consumer's build, in
  keeping with Oakestra's lightweight-first design principle.

The models get generated twice, once per module, instead of shared: importing the server's
`openapi` package would drag its whole dependency graph along, which is exactly what the separate
`go.mod` avoids. Both copies come from the same spec, so they can't disagree.

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

    candidates, err := c.Resources.List(ctx, client.Active())
    if err != nil {
        log.Fatal(err)
    }
    for _, candidate := range candidates {
        log.Println(client.Value(candidate.CandidateName))
    }

    job, err := c.Jobs.GetByID(ctx, "507f1f77bcf86cd799439011")
    switch {
    case errors.Is(err, client.ErrNotFound):
        // no such job
    case err != nil:
        // transport error, or a non-2xx/404 response (*client.APIError)
    default:
        log.Println(client.Value(job.JobName))
    }
}
```

`client.New(baseURL, opts...)` builds a client against an explicit base URL (no `/api/v1` suffix -
the spec's paths carry it) when you don't want to read it from the environment.
`RESOURCE_ABSTRACTOR_URL` itself may be a bare host or already carry a `http://`/`https://` scheme
- `NewFromEnv` uses either form as given. `WithHTTPClient` and `WithTimeout` (default 10s) are
available as `Option`s. `WithHTTPClient` takes a concrete `*http.Client`; to point requests at a
fake instead of the network, install a custom `http.RoundTripper` on that client rather than
reaching for the generated client directly (see "Testing" below).

### Filtering lists

`Apps.List`, `Resources.List` and `Jobs.List` take filter options rather than a query struct.
Each one maps to a query parameter the spec declares, and passing none lists everything:

```go
jobs, err := c.Jobs.List(ctx, client.OfApplication(appID))
apps, err := c.Apps.List(ctx, client.NamedApp("nginx"), client.InNamespace("default"))

// Extend the canonical projection to read fields the service stores but
// doesn't return by default. The spec sends them as one comma-separated
// value; pass them separately and let the generated encoder join them.
res, err := c.Resources.List(ctx, client.Active(), client.ResourceFields("gpu_temp"))
```

The filter types are distinct per resource (`AppFilter`, `ResourceFilter`, `JobFilter`), so
handing a job filter to `Resources.List` doesn't compile.

`Hooks.List` takes no filters at all - `listHooks` declares no query parameters in the spec, so
there is nothing for a `HookFilter` to build.

### Optional fields

Every field of every document is a pointer, because the service stores partial documents and
patches them field by field - nothing can be declared required in the spec. Two helpers keep that
from leaking into call sites:

```go
job, err := c.Jobs.Create(ctx, client.Job{JobName: client.Ptr("my-job")})

name := client.Value(job.JobName)   // "" when the service didn't set it
```

Prefer `client.Value(job.JobName)` to `*job.JobName`, which panics on the first document that
omits the field - and on a service that stores whatever it is given, that is any of them.

### Documents are schemaless

Every collection the abstractor fronts is a pass-through to MongoDB: unknown fields are stored
verbatim and returned again on reads, which is why every document schema in the spec sets
`additionalProperties`. The models carry that through in an `AdditionalProperties` map, reachable
with `Get`/`Set`:

```go
job.Set("status", "RUNNING")           // a field the spec doesn't declare

if temp, ok := candidate.Get("gpu_temp"); ok {
    // ...
}
```

Treat the declared struct fields as what consumers can rely on, not as an exhaustive list of what
a stored document contains.

`Hook` is the one exception: its schema sets no `AdditionalProperties`, so a hook registration is
exactly the fields the spec declares - nothing else can be stored alongside them, and there is no
`Get`/`Set` on it.

### Error handling

Every method returns `(value, error)`. That's a deliberate improvement over the Python client,
which returns `None` for a 404, a connection failure, and any other non-2xx response alike, so
callers can't tell "not found" apart from "the request failed":

- `errors.Is(err, client.ErrNotFound)` - the abstractor returned 404, whichever method was called
  (`GetByID`, `List`, `Delete`, or anything else). `GetByName`/`GetByIP`/`GetByNameAndNamespace`
  also return it when the underlying list came back empty (mirroring the Python client's
  `result[0] if result else None` pattern), since the abstractor never answers those with a 404
  itself.
- `errors.As(err, &apiErr)` where `apiErr` is `*client.APIError` - any other non-2xx response.
  Carries `Status`, the request's method and path, and `Message` when the abstractor's
  `{"message": "..."}` error body could be decoded.
- Any other `error` - a transport-level failure (DNS, connection refused, context
  cancellation/deadline), or a 2xx body that wasn't the JSON the spec promises.

The status always decides, whatever the body turns out to contain, so a proxy answering with an
empty 404 is still `ErrNotFound`. That holds past the facade too: `Client.OpenAPI()` hands back
only the low-level generated operations, each returning a bare `(*http.Response, error)`, and
`Decode`/`Done` map those onto the same three-way split, so the contract holds across the whole
surface rather than just the facaded part:

```go
defs, err := client.Decode[[]client.CustomResourceDefinition](
    c.OpenAPI().ListCustomResourceDefinitions(ctx))

err := client.Done(c.OpenAPI().DeleteCustomResourceInstance(ctx, "gpu", id))
```

A list is expressed through the type parameter (`Decode[[]client.CustomResourceDefinition]`)
rather than a separate function, so one generic covers both single documents and lists. The
`...WithResponse` variants the generator also produces are deliberately unreachable through
`OpenAPI()`: handing those out would let this escape hatch silently opt out of the contract the
rest of the package promises.

## API surface

`Apps`, `Resources` and `Jobs` cover the endpoints the Python `resource_abstractor_client`
actually has consumers for, including the job instance sub-routes. `Hooks` is new surface area
with no Python equivalent to port - `resource_abstractor_client` never covered hooks - but the
same shape as the other three: one concept, five operations (`List`, `GetByID`, `Create`,
`Update`, `Delete`), reachable as `c.Hooks`:

```go
hooks, err := c.Hooks.List(ctx)
```

See `applications.go`, `resources.go`, `jobs.go` and `hooks.go` for the full method list (Python
equivalents documented per-method where one exists), and `models.go` for the re-exported types.

Custom resources have no facade. Reaching them goes through `OpenAPI()` plus `Decode`/`Done` (see
"Error handling" above) rather than a `CustomResourcesService`.

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

A downstream service testing its own code against a fake abstractor doesn't need to stub HTTP at
that level: installing a custom `http.RoundTripper` on the `*http.Client` passed to
`WithHTTPClient` routes every request through it while keeping the whole facade intact - error
contract, filter options, aliased models - rather than forcing a drop down to the generated
client.

```go
c := client.New("http://stub", client.WithHTTPClient(
    &http.Client{Transport: fakeRoundTripper{}}))
```

`WithTimeout` still applies on top of this, since what `New` copies is a real `*http.Client`.

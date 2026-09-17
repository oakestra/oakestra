# go_resource_abstractor

A Go port of [`resource-abstractor`](../resource-abstractor), Oakestra's REST API in front of
MongoDB for reading/writing resource state (candidates, jobs, apps, webhooks, custom resources).
It exposes the same API surface as the Python service so it can be swapped in as a drop-in
replacement at either the root (`root_resource_abstractor`, port 11011) or cluster
(`cluster_resource_abstractor`, port 11012) level, differentiated purely by env vars.

Docker-compose still defaults to the Python service. To deploy this Go port instead, opt in
with the composable override (which just swaps the abstractor's build/image; port, env vars and
dependencies are inherited):

```bash
export OVERRIDE_FILES=override-go-resource-abstractor.yml
./scripts/StartOakestraRoot.sh      # or StartOakestraCluster.sh / StartOakestraFull.sh
```

The override lives at `root_orchestrator/override-go-resource-abstractor.yml` and
`cluster_orchestrator/override-go-resource-abstractor.yml`.

## Why a Go port

The Python/Flask service is fine at small scale but carries the overhead of Flask, marshmallow
schema validation, and pymongo's synchronous driver. This port aims for a smaller memory
footprint and lower request latency while preserving the exact HTTP contract, so no consumer
(the Go `scheduler`, or the Python `system_manager`/`cluster_manager` via
`resource_abstractor_client`) needs to change.

## Building and running

```bash
go build -o resource_abstractor ./cmd/resource-abstractor

export RESOURCE_ABSTRACTOR_PORT=11011
export MONGO_URL=localhost
export MONGO_PORT=10007
./resource_abstractor
```

### Tests

The `rest`, `internal/store` and `abstractor` packages test against a real MongoDB via
[testcontainers-go](https://golang.testcontainers.org/), so running the full suite needs Docker
available locally or in CI.

```bash
go test ./...
```

`internal/hooks` and `model` do not need Docker: `hooks.Hooks` reaches storage through the
one-method `HookRegistry` interface, so its tests substitute an in-memory registry, and `model`'s
tests are pure JSON/BSON round-trips.

```bash
go test ./internal/hooks/ ./model/    # no Docker required
```

The test MongoDB image defaults to `mongo:8.0` (matching the rest of Oakestra's deployment). On
hosts whose kernel is incompatible with MongoDB 8.0's storage engine, override it:

```bash
MONGO_TEST_IMAGE=mongo:7.0 go test ./...
```

### Docker image

```bash
docker build -t resource_abstractor .
```

Builds a static binary into a `distroless/static` runtime image - no shell, no package manager,
nonroot user, minimal attack surface and image size.

## Configuration

| Env var | Default | Purpose |
|---|---|---|
| `RESOURCE_ABSTRACTOR_PORT` | *(required)* | HTTP listen port. |
| `MONGO_URL` | *(required)* | MongoDB host. |
| `MONGO_PORT` | *(required)* | MongoDB port. |
| `LOG_LEVEL` | `DEBUG` | `DEBUG`, `INFO`, `WARN`/`WARNING`, or `ERROR`. |
| `HOOK_CONNECT_TIMEOUT` | `10` (seconds) | Connect timeout for outbound webhook calls. |
| `HOOK_REQUEST_TIMEOUT` | `5` (seconds) | Response timeout for outbound webhook calls. |

## API surface

[`openapi/openapi.yaml`](openapi/openapi.yaml) is the contract, and it is the source of truth
rather than documentation written after the fact:
[oapi-codegen](https://github.com/oapi-codegen/oapi-codegen) turns it into the route table, the
typed path/query parameters, the response models, and a `ServerInterface` the handlers in `rest/`
implement - so an endpoint that isn't in the spec doesn't exist, and one that is in the spec but
unimplemented fails the build.

The same file also generates the other direction: [`client/`](client), the Go client for this
service, is generated from it too - so the client's paths, parameter encoding and models can't
drift from what the server actually serves. See [`client/README.md`](client/README.md).

The running service publishes it at `GET /docs/openapi.json` (the path the Python service used)
and `GET /docs/openapi.yaml`. Point any Swagger UI / Redoc at either.

Two deliberate exceptions to full codegen, both commented where they matter:

- **Request bodies are not generated.** The spec's plain (non-strict) gin server binds
  parameters only. Binding bodies into typed structs would drop the unknown fields this service
  is required to store verbatim, so the handlers keep decoding them into generic maps.
- **`?active=` and `?instance_number=` are typed as strings**, not `boolean`/`integer`. Both
  carry a validation contract inherited from the Python service - marshmallow's boolean spellings
  (`yes`, `on`, `t`, ...) and a 422 rather than 400 on a bad value - that generated binding
  cannot reproduce. They are parsed inside the handlers instead.

### Regenerating

Editing `openapi/openapi.yaml` means regenerating both halves - the server here and the client
in its own module:

```bash
go generate ./openapi             # server:  openapi/openapi.gen.go
go generate -C client ./openapi   # client:  client/openapi/openapi.gen.go
```

A pre-commit hook runs both when the spec changes. Both generated files are committed, so
building or deploying never runs a code generator; CI fails if either drifts from the spec. The
generator version is pinned in the `go:generate` directives (`openapi/spec.go` and
`client/openapi/gen.go`), which have to stay on the same version as each other.

### Routes

All routes are under `/api/v1`, plus `GET /` for a plain-text `ok` health check and the `/docs`
endpoints above:

- `/resources` - candidate (worker/cluster) resource records, backed by an aggregation that
  computes freshness (`active`, a 30s window) and a projection controllable via `?resources=csv`.
- `/applications` - application metadata.
- `/jobs` - jobs and their per-worker instances (`/jobs/<id>/<instance>`), including cpu/memory
  history tracking capped at the most recent 100 samples.
- `/hooks` - webhook registrations that fire around writes to the above (and custom resources):
  `pre_create`/`pre_update` run synchronously and may transform the payload before it's
  persisted; `post_create`/`post_update`/`post_delete` fire asynchronously after the write.
- `/custom-resources` - dynamically registered resource types (each with a JSON Schema) and
  their instances, stored in per-type collections.

See [`../resource-abstractor/README.md`](../resource-abstractor/README.md) for the original
service description; the endpoint shapes, MongoDB layout (four databases: `candidates`, `jobs`,
`hooks`, `custom_resources`), and business logic are preserved here field-for-field, with several
deliberate bug fixes over the Python source (documented in code comments where they matter):

- `GET /hooks/<id>` now actually finds the hook by its ObjectID (the Python service compared it
  as a raw string, so that lookup never matched).
- `PUT /jobs/` fires webhooks under the `"jobs"` entity name on its update path, consistent with
  every other job route (the Python service used `"job"`, singular, there only).
- A malformed id in a path parameter answers 400 on every route except `PATCH /resources/<id>`
  and `GET /hooks/<id>`, which keep answering 404 (a deliberate, documented exception). The Python
  service (and the first cut of this port) only pre-checked some routes; on the rest the id
  reached the driver and the request 500'd.
- Registering a custom resource type named `meta_data`, using a `system.` prefix, containing `$`
  or a NUL byte, or longer than MongoDB's 120-byte collection-name limit is rejected with 400 -
  and not just on registration: every operation under `/custom-resources/{resource}` re-validates
  the name shape, not only create. Instances of a type live in a collection named after the type,
  in the same database as the `meta_data` definitions collection, so a type with that name pointed
  at the definitions themselves - deleting it wiped every registered type.
- `GET /custom-resources/{resource}` rejects any filter query key starting with `$` (400) instead
  of passing it through as a MongoDB filter verbatim - unvalidated, a key like `$where` would
  otherwise reach the query as an operator.
- A JSON value of the wrong type for a typed field (e.g. `{"job_name": 5}`, where `job_name` is a
  string) now answers 400 instead of being stored verbatim the way the Python service, and every
  field that isn't in `openapi.yaml`'s narrow set of validated ones, would. Typed fields decode via
  `model`'s own `UnmarshalJSON` (see `rest/common.go`'s `decodeModel`), and a Go type mismatch
  there is as malformed a request as invalid JSON, from the caller's point of view. Unknown fields
  are unaffected - they still land in `Extra` and round-trip untouched regardless of their shape.
- A synchronous (`pre_create`/`pre_update`) webhook whose response body doesn't decode into the
  entity's model type - a JSON type mismatch on a typed field, most likely - now fails the write
  with 500, instead of the transformed-but-invalid payload being stored verbatim the way a
  `bson.M` write path would happily accept anything shaped like a document. The hook still runs
  and its response is fetched exactly as before (see `internal/hooks`'s fail-open behavior for
  network/non-2xx/non-JSON failures, which is unchanged); only a response that *is* valid JSON but
  doesn't fit the model changes the outcome, since `abstractor`'s `fromMap` (see
  `abstractor/convert.go`) now has to decode it back into a typed value before the write can
  proceed.

The one route not carried over is `/api/docs`, the Swagger UI page the Python service bundled.
The spec it rendered is still served (see above); serving the UI itself would mean shipping its
static assets, or fetching them from a CDN that an edge deployment may not be able to reach.

## Package layout

```
go_resource_abstractor/
├── cmd/resource-abstractor/  # entrypoint: config -> mongo connect -> abstractor.New -> rest.NewHandler -> graceful shutdown
├── abstractor/                # PUBLIC library: typed CRUD + webhook choreography + validation (Service, Apps/Jobs/Resources/Hooks/CustomResources)
├── model/                     # PUBLIC typed documents (Application, Job, Resource, Hook, CustomResource...), no gin/mongo-driver dependency in the public surface
├── rest/                       # thin gin transport over abstractor.Service, implementing openapi.ServerInterface
├── internal/store/            # MongoDB access layer (one file per collection group), typed in/out via model
├── internal/hooks/            # webhook dispatcher (internal/hooks.Hooks), used by abstractor
├── internal/config/           # env var loading + slog setup, used only by cmd/resource-abstractor
├── internal/errs/             # sentinel errors and ValidationError shared by store/hooks/abstractor
└── openapi/                   # openapi.yaml (the contract) + the code generated from it
```

`abstractor` and `rest` are the two packages an external caller ever needs: `abstractor` for
in-process use (see "Using as a library" below), `rest` to mount the same HTTP surface this
binary serves standalone. Everything under `internal/` is a private implementation detail of
`abstractor` and `cmd/resource-abstractor`.

## Using as a library

Once the root/cluster orchestrator is ported to Go, it can skip the HTTP hop entirely and call
into `abstractor.Service` directly with a `*mongo.Client` it already owns:

```go
import (
	"context"
	"log/slog"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/oakestra/oakestra/go_resource_abstractor/abstractor"
	"github.com/oakestra/oakestra/go_resource_abstractor/model"
)

func run(ctx context.Context) error {
	// The caller dials and owns the client - abstractor.New never calls
	// Connect, Ping or Disconnect on it.
	client, err := mongo.Connect(options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		return err
	}
	defer client.Disconnect(context.Background())

	svc, err := abstractor.New(abstractor.Options{
		Client:             client,
		HookConnectTimeout: 10 * time.Second,
		HookRequestTimeout: 5 * time.Second,
		Logger:             slog.Default(),
	})
	if err != nil {
		return err
	}
	// Drains in-flight post_* webhooks; never touches client - Disconnect
	// above is the caller's job, and should run after this returns.
	defer svc.Close(context.Background())

	if err := svc.EnsureIndexes(ctx); err != nil {
		return err
	}

	job, err := svc.Jobs.Create(ctx, model.Job{JobName: model.Ptr("nginx")})
	if err != nil {
		return err
	}
	_ = job
	return nil
}
```

Every sub-service (`svc.Apps`, `svc.Jobs`, `svc.Resources`, `svc.Hooks`,
`svc.CustomResources`) takes and returns `model` types - no `bson.M`, no gin. Pre/post webhook
choreography, upsert-by-name, and the validation rules documented in code comments (resource type
name validation, hook event validation, custom resource JSON Schema validation) all run the same
way whether the call came from `rest` or directly from library code. Errors are the sentinels in
`abstractor/errors.go` (`ErrNotFound`, `ErrInvalidID`, `ErrInvalidResourceType`, ...); check them
with `errors.Is`.

Every optional field on a `model` type is a plain pointer (`*string`, `*int64`, `*[]T`, ...);
`model.Ptr(v)` builds one from a literal. `nil` means absent, so it's left out of the JSON a
write sends and a partial update (`Jobs.Update`, `Apps.Update`, ...) leaves the stored field
untouched. A non-nil pointer is a real value even when it points at a zero number or an empty
slice, so `model.Ptr([]string{})` still round-trips as a present, empty array. An explicit JSON
`null` on the wire is treated the same as an absent field.

An embedder that still wants the HTTP surface - to keep serving `resource_abstractor_client`
callers while also using the library directly - can mount `rest.NewHandler(svc, logger)` as an
`http.Handler` in its own server instead of running this module's binary.

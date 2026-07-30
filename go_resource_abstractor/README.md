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
go build -o resource_abstractor .

export RESOURCE_ABSTRACTOR_PORT=11011
export MONGO_URL=localhost
export MONGO_PORT=10007
./resource_abstractor
```

### Tests

Tests run against a real MongoDB via [testcontainers-go](https://golang.testcontainers.org/) -
Docker must be available locally or in CI.

```bash
go test ./...
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
typed path/query parameters, the response models, and a `ServerInterface` the handlers in `api/`
implement - so an endpoint that isn't in the spec doesn't exist, and one that is in the spec but
unimplemented fails the build.

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

```bash
go generate ./openapi     # after editing openapi/openapi.yaml
```

`openapi/openapi.gen.go` is committed, so building or deploying the service never runs a code
generator; CI fails if the two drift apart. The generator version is pinned in the `go:generate`
directive in `openapi/spec.go`.

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
`hooks`, `custom_resources`), and business logic are preserved here field-for-field, with two
deliberate bug fixes over the Python source (documented in code comments where they matter):

- `GET /hooks/<id>` now actually finds the hook by its ObjectID (the Python service compared it
  as a raw string, so that lookup never matched).
- `PUT /jobs/` fires webhooks under the `"jobs"` entity name on its update path, consistent with
  every other job route (the Python service used `"job"`, singular, there only).

The one route not carried over is `/api/docs`, the Swagger UI page the Python service bundled.
The spec it rendered is still served (see above); serving the UI itself would mean shipping its
static assets, or fetching them from a CDN that an edge deployment may not be able to reach.

## Package layout

```
go_resource_abstractor/
├── main.go        # entrypoint: config -> mongo connect -> router -> graceful shutdown
├── config/        # env var loading
├── logger/        # log/slog setup
├── openapi/       # openapi.yaml (the contract) + the code generated from it
├── db/            # MongoDB access layer (one file per collection group)
├── services/      # webhook dispatch (services.Hooks)
└── api/           # HTTP handlers implementing openapi.ServerInterface
```

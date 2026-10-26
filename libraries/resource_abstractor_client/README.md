# resource_abstractor_client

A Python HTTP client for the resource abstractor (`resource-abstractor` and its Go port,
[`go_resource_abstractor`](../../go_resource_abstractor), which serve the same `/api/v1` REST API).
Used by `root_orchestrator/system-manager-python` and `cluster_orchestrator/cluster-manager`.

## Generated from the service's own spec

The request-building half of this package -
[`resource_abstractor_client/openapi/`](resource_abstractor_client/openapi/) - is generated from
[`go_resource_abstractor/openapi/openapi.yaml`](../../go_resource_abstractor/openapi/openapi.yaml),
the same single file the Go server and its own Go client
([`go_resource_abstractor/client`](../../go_resource_abstractor/client)) are generated from:

```
                       openapi/openapi.yaml
                  /              |               \
     gin ServerInterface   Go ClientInterface   Python operation modules
  (go_resource_abstractor)  (client/openapi)   (resource_abstractor_client/openapi)
```

[openapi-python-client](https://github.com/openapi-generators/openapi-python-client) reads the spec
and emits one module per `operationId` (`resource_abstractor_client/openapi/api/jobs/upsert_job.py`,
...), each with a `_get_kwargs()` function that builds the method, URL, query string and body for
that operation. A spec change that renames a query parameter or adds an endpoint changes this output
automatically the next time it's regenerated - the same property the Go side already has.

### Regenerating

```bash
python scripts/generate_python_client.py     # after editing ../../go_resource_abstractor/openapi/openapi.yaml
```

`resource_abstractor_client/openapi/` is committed, so installing this package never runs a code
generator. A pre-commit hook regenerates it whenever the spec changes (alongside the two Go
generation hooks, which read the same file); CI fails if the committed output has drifted. The
generator version is pinned in `.pre-commit-config.yaml` and in `scripts/generate_python_client.py`
and must stay in step with the one CI uses.

## Transport only - this package doesn't parse responses

`app_operations.py`, `candidate_operations.py` and `job_operations.py` keep the exact function
signatures this package has always had (`get_apps(**kwargs)`, `get_job_by_id(job_id, filter={})`,
...) - callers across `system_manager` and `cluster_manager` don't change. Internally, each function
builds a request with the matching generated `_get_kwargs()` and sends it through
[`client_helper.make_request`](resource_abstractor_client/client_helper.py), which returns the
decoded JSON body directly - a `dict` or a `list`, never one of the generated
`resource_abstractor_client.openapi.models` classes.

This is deliberate, and mirrors the reasoning in the Go client's
[`response.go`](../../go_resource_abstractor/client/response.go): the resource abstractor is a
schemaless Mongo pass-through (`additionalProperties: true` on every document schema), so a document
can carry fields the spec doesn't describe - `microserviceID`, `next_instance_progressive_number`,
`cluster_id`, and others that `system_manager`/`cluster_manager` read directly. Parsing into a typed
model and handing back that model (or `.to_dict()` of it) risks losing fields the model wasn't told
about; returning the raw decoded body never does.

The generated models are still used, just not for responses: a request body like `create_job(data)`
is sent as `Job.from_dict(data).to_dict()` via the operation module's `_get_kwargs(body=...)`, which
gets the correct path-parameter quoting and `Content-Type` header from the generated code while still
round-tripping every field in `data`, known or not.

## Error handling: unchanged from before this port

Every function returns `None` for a 404, a connection failure, or any other non-2xx response alike -
callers' existing `if result is None:` checks keep working. This is *not* the same contract as the Go
client, which distinguishes `ErrNotFound` from `*APIError` from a transport error; matching that here
would mean revisiting every caller, which is out of scope for this change. See
[`client_helper.make_request`](resource_abstractor_client/client_helper.py).

## Filters: still `**kwargs`, now validated against the spec

`get_apps`, `get_candidates` and `get_jobs` still take arbitrary `**kwargs` and forward them as query
parameters, as before. Internally, `client_helper.translate_kwargs` maps a caller's wire-named kwarg
(`userId`, `applicationID`, ...) onto the generated function's Python parameter name and drops
anything the spec doesn't declare, logging a warning instead of the parameter silently vanishing into
a malformed query string. This only becomes visible for arguments the spec has never actually
supported - see `job_management.py`'s `mark_inactive_as_failed`/`get_jobs_with_failed_instances`, which pass
Mongo filter documents that were dropped exactly the same way before, just silently.

## What's not covered

Hooks and custom resources - the resource abstractor's newer endpoints - have no operations module
here, matching the state of this client before this port (`go_resource_abstractor/client` covers
both; this package never did). Adding one is now a small amount of code: wrap the relevant generated
modules in `resource_abstractor_client/openapi/api/hooks/` the way `app_operations.py` wraps
`api/applications/`.

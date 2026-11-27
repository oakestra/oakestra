### Shared Libraries

The libraries in this directory (`oakestra-utils`, `resource-abstractor-client`, and `oakestra-logging`) are members of the root `uv` workspace defined in [pyproject.toml](../pyproject.toml).

#### Local Development:
Run `uv sync --all-packages` from the repository root to install the shared libraries in editable mode into the workspace `.venv`:

```bash
uv sync --all-packages
```

#### In Docker builds:
The service Dockerfiles copy `pyproject.toml`, `uv.lock`, and `libraries/` into the build container and install dependencies via `uv sync`.

## Available libraries

- `oakestra_utils_library` contains shared status and scheduling types.
- `resource_abstractor_client` contains the Resource Abstractor HTTP client.
- `oakestra_logging` defines the versioned structured JSON logging contract used by Python
  orchestrator services. See its [README](./oakestra_logging/README.md) for API and schema details.

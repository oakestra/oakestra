### Shared Libraries

The libraries in this directory (`oakestra-utils`, `resource-abstractor-client`) are members of the root `uv` workspace defined in [pyproject.toml](../pyproject.toml).

#### Local Development:
Run `uv sync --all-packages` from the repository root to install the shared libraries in editable mode into the workspace `.venv`:

```bash
uv sync --all-packages
```

#### In Docker builds:
The service Dockerfiles copy `pyproject.toml`, `uv.lock`, and `libraries/` into the build container and install dependencies via `uv sync`.

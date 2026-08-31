Contributing to Oakestra
========================

First of all, welcome to Oakestra! We are happy that you are interested in
contributing to this project.  New features, bug fixes, improvements,
maintenance and everything in between as contributions are welcome!

The Oakestra project is an open source project and encourages the fostering open
collaboration.  For details on how to contribute to the Unikraft project,
[please read the contribution
guidelines](https://www.oakestra.io/docs/contribute/) located on the
[Wiki website](https://www.oakestra.io/docs/).

## Development Setup

### 1. Core Services & UV Workspace

Oakestra uses a [`uv`](https://docs.astral.sh/uv/) workspace at the repository root to manage dependencies for core Python services and internal libraries:
- `root_orchestrator/system-manager-python`
- `cluster_orchestrator/cluster-manager`
- `libraries/oakestra-utils`
- `libraries/resource-abstractor-client`

After cloning, install and synchronize the root virtual environment:

```bash
uv sync
```

Running `uv sync` installs all workspace packages and links shared libraries in editable mode into the root `.venv/`.

### 2. Non-UV Python Services (Dedicated Subfolder Environments)

Services that are not yet migrated to `uv` (`resource-abstractor`, `jwt-generator`, `addons_engine/*`, `marketplace-manager`) maintain their own `requirements.txt`.

> [!IMPORTANT]
> **Do not install `requirements.txt` into the root `.venv`.**
> Running `uv sync` strictly reconciles the root `.venv` against `uv.lock` and will uninstall packages added via `pip`. Mixing pip packages into the root virtualenv also leads to dependency conflicts.

To develop or debug a non-uv service locally outside of Docker:
1. Navigate to the service directory.
2. Create an isolated virtual environment inside that subfolder.
3. Install its dependencies:

```bash
cd resource-abstractor
python3 -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
```

*(Note: If you use VS Code, pre-configured tasks like `install-resource-abstractor-dependencies` and debug launch targets automatically create and use these subfolder `.venv` environments.)*

### 3. Git hooks (pre-commit)

This repository uses [pre-commit](https://pre-commit.com) to run linting and
code generation automatically. Install `pre-commit` globally on your system (e.g., using `uv tool` or your system Python, rather than into a local project virtual environment):

```bash
# Recommended: install as a global tool
uv tool install pre-commit

# Or via pip on your system interpreter:
pip install pre-commit

# Install the repository hooks
pre-commit install
```

The `pre-commit install` command installs all required hook types, including
`post-checkout` and `post-merge`, which automatically regenerate protobuf
bindings whenever you switch branches or pull changes.

### 4. Protobuf files

The generated protobuf files (`*_pb2.py`, `*_pb2.pyi`, `*_pb2_grpc.py`) are
not committed to the repository. They are regenerated automatically by the git
hooks above. To regenerate them manually, run:

```bash
python3 scripts/generate_protos.py
```

This automatically invokes `grpc_tools.protoc` within the `uv` workspace environment.
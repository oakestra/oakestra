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

### Git hooks (pre-commit)

This repository uses [pre-commit](https://pre-commit.com) to run linting and
code generation automatically. Install it once after cloning:

```bash
pip install pre-commit
pre-commit install
```

The `pre-commit install` command installs all required hook types, including
`post-checkout` and `post-merge`, which automatically regenerate protobuf
bindings whenever you switch branches or pull changes.

### Protobuf files

The generated protobuf files (`*_pb2.py`, `*_pb2.pyi`, `*_pb2_grpc.py`) are
not committed to the repository. They are regenerated automatically by the git
hooks above. To regenerate them manually, run:

```bash
python scripts/generate_protos.py
```

This requires `grpcio-tools` to be installed (`pip install grpcio-tools`).

### OpenAPI files

Go services that expose a REST API describe it in an OpenAPI spec, and generate
their routing, parameter binding and models from it - currently
`go_resource_abstractor/openapi/openapi.yaml`. Editing a spec triggers a
pre-commit hook that regenerates the corresponding Go code; if the result
differs from what you staged, the commit fails and you re-stage it.

Unlike the protobuf bindings, the generated Go code **is** committed, so
building or deploying a service never runs a code generator, and no
post-checkout hook is needed. CI verifies the two stay in step.

To regenerate manually:

```bash
go generate -C go_resource_abstractor ./openapi
```

This requires the [Go toolchain](https://go.dev/dl/); the generator itself is
pinned in the service's `go:generate` directive and fetched on first use, so
there is nothing else to install.
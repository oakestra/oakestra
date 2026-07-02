#!/usr/bin/env python3
"""Generate protobuf Python bindings for all .proto files in the repo."""

import subprocess
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent

PROTO_DIRS = [
    REPO_ROOT / "cluster_orchestrator" / "cluster-manager" / "proto",
    REPO_ROOT / "root_orchestrator" / "system-manager-python" / "proto",
]


def generate(proto_dir: Path) -> None:
    proto_files = list(proto_dir.glob("*.proto"))
    if not proto_files:
        return
    service_root = proto_dir.parent
    print(f"Generating protobuf files in {proto_dir.relative_to(REPO_ROOT)}")
    subprocess.run(
        [
            sys.executable,
            "-m",
            "grpc_tools.protoc",
            "-I.",
            "--python_out=.",
            "--pyi_out=.",
            "--grpc_python_out=.",
            *[str(Path(proto_dir.name) / f.name) for f in proto_files],
        ],
        check=True,
        cwd=service_root,
    )


if __name__ == "__main__":
    for proto_dir in PROTO_DIRS:
        generate(proto_dir)

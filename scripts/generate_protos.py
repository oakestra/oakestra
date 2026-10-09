"""Generate protobuf Python bindings for all .proto files in the repo."""

import subprocess
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent

SERVICES = [
    (
        REPO_ROOT / "cluster_orchestrator" / "cluster-manager",
        "src/cluster_manager/proto/cluster_registration.proto",
    ),
    (
        REPO_ROOT / "root_orchestrator" / "system-manager-python",
        "src/system_manager/proto/cluster_registration.proto",
    ),
]


def generate(service_dir: Path, proto_rel_path: str) -> None:
    proto_file = service_dir / proto_rel_path
    if not proto_file.exists():
        return
    print(f"Generating protobuf files for {proto_file.relative_to(REPO_ROOT)}")
    subprocess.run(
        [
            "uv",
            "run",
            "-m",
            "grpc_tools.protoc",
            "-Isrc",
            "--python_out=src",
            "--pyi_out=src",
            "--grpc_python_out=src",
            proto_rel_path,
        ],
        check=True,
        cwd=service_dir,
    )


if __name__ == "__main__":
    for service_dir, proto_rel_path in SERVICES:
        generate(service_dir, proto_rel_path)

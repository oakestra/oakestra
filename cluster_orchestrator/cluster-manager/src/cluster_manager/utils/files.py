import os
from pathlib import Path


def write_file_atomic(path, data, mode: int) -> None:
    """Replace path with data in one step, so a reader never sees a half-written file."""
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    if isinstance(data, str):
        data = data.encode("utf-8")
    tmp_path = path.with_name(f".{path.name}.tmp")
    tmp_path.write_bytes(data)
    os.chmod(tmp_path, mode)
    os.replace(tmp_path, path)

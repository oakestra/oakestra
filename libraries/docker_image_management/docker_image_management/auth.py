"""Resolve docker registry credentials from the operator's docker config.

The Python `docker` SDK does NOT read ~/.docker/config.json on its own — it
sends no X-Registry-Auth header to the daemon and the daemon falls back to
anonymous pulls. Oakestra services that need to pull private images mount the
host's ~/.docker/config.json into the container and use this resolver to
extract credentials at pull time.

Credential helpers (credsStore / credHelpers) are NOT supported: those need
the helper binary inside the container, which Oakestra service images don't
ship. Operators using a helper should re-login with --password-stdin while
the helper is disabled so credentials land directly in `auths.<reg>.auth`.
"""

import base64
import binascii
import json
import os

DEFAULT_DOCKER_CONFIG_PATH = "~/.docker/config.json"


def _registry_from_image(image):
    """Extract the registry hostname from a Docker image reference.

    Rule of thumb: if the first slash-separated segment contains '.' or ':',
    or equals 'localhost', it's a registry. Otherwise the image lives on
    Docker Hub (docker.io).
    """
    first, _, _ = image.partition("/")
    if "/" not in image:
        return "docker.io"
    if "." in first or ":" in first or first == "localhost":
        return first
    return "docker.io"


def _read_docker_config(path):
    try:
        with open(path) as f:
            return json.load(f)
    except (OSError, ValueError):
        # OSError covers FileNotFoundError, IsADirectoryError, PermissionError.
        # ValueError covers json.JSONDecodeError (its parent class).
        # When the host's ~/.docker/config.json doesn't exist, Docker bind
        # mounts auto-create an empty directory at the source path — so
        # inside the container the mount point is a directory, not a file.
        return None


def _entry_for_registry(auths, registry):
    candidates = [
        registry,
        f"https://{registry}",
        f"https://{registry}/",
        f"https://{registry}/v1/",
    ]
    if registry == "docker.io":
        candidates.append("https://index.docker.io/v1/")
    return next((auths[k] for k in candidates if k in auths), None)


def _decode_auth_field(auth_b64):
    if not isinstance(auth_b64, str) or not auth_b64:
        return None
    try:
        decoded = base64.b64decode(auth_b64).decode("utf-8", errors="replace")
    except (ValueError, binascii.Error):
        return None
    user, sep, pwd = decoded.partition(":")
    if not (sep and user and pwd):
        return None
    return {"username": user, "password": pwd}


def auth_config_for_image(image, docker_config_path=None):
    """Return an auth_config dict for pulling `image`, or None for anonymous.

    None is returned whenever credentials are unavailable or the config can't
    be read — public images keep working, private images fail at pull time
    with the registry's own 401, which is the right signal for an operator.
    """
    path = os.path.expanduser(
        docker_config_path
        or os.environ.get("DOCKER_CONFIG_PATH", DEFAULT_DOCKER_CONFIG_PATH)
    )
    cfg = _read_docker_config(path)
    if not isinstance(cfg, dict):
        return None
    auths = cfg.get("auths")
    if not isinstance(auths, dict):
        return None

    entry = _entry_for_registry(auths, _registry_from_image(image))
    if not isinstance(entry, dict):
        return None

    return _decode_auth_field(entry.get("auth")) or _explicit_user_pass(entry)


def _explicit_user_pass(entry):
    user = entry.get("username")
    pwd = entry.get("password")
    if isinstance(user, str) and isinstance(pwd, str) and user and pwd:
        return {"username": user, "password": pwd}
    return None

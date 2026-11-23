"""Auth-aware wrapper around docker.images.pull."""

from .auth import auth_config_for_image


def pull_image(client, image):
    """Pull `image` using credentials from the host's docker config.

    `client` is a docker.DockerClient (typically docker.from_env()). Returns
    the resulting Image object. Raises whatever the SDK raises on failure;
    callers handle that with a try/except — docker.errors.DockerException.
    """
    return client.images.pull(image, auth_config=auth_config_for_image(image))

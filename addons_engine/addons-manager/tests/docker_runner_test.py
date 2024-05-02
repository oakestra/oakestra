import sys
import uuid
from pathlib import Path

import docker
import pytest
import tests.container_utils as container_utils

current_dir = Path(__file__).parent
parent_dir = current_dir.parent
sys.path.append(str(parent_dir))

from addons_runner.docker_runner import (  # noqa: E402
    ADDONS_ID_LABEL,
    ADDONS_MANAGER_LABEL,
    ADDONS_SERVICE_NAME_LABEL,
    DEFAULT_NETWORK_NAME,
    DockerRunner,
)


@pytest.fixture(scope="session")
def docker_client():
    return docker.from_env()


def test_run_addon_creates_container(docker_client):
    addons_manager_id = str(uuid.uuid4())
    docker_manager = DockerRunner(addons_manager_id)

    test_addon = container_utils.get_dummy_addon()
    result = docker_manager.run_addon(test_addon)
    assert result.get("new_containers") is not None
    assert len(result.get("new_containers")) == 1

    created_container = result.get("new_containers", [])[0]

    assert created_container is not None
    assert DEFAULT_NETWORK_NAME in created_container.attrs["NetworkSettings"]["Networks"]

    labels = created_container.labels
    assert labels[ADDONS_ID_LABEL] == test_addon["_id"]
    assert labels[ADDONS_MANAGER_LABEL] == addons_manager_id
    assert labels[ADDONS_SERVICE_NAME_LABEL] == test_addon["services"][0]["service_name"]

    label = f"{ADDONS_MANAGER_LABEL}={addons_manager_id}"
    oak_containers = docker_client.containers.list(filters={"label": label}, all=all)
    assert len(oak_containers) == len(test_addon["services"])
    assert oak_containers[0].id == created_container.id

    # Cleanup
    docker_manager.stop_addon(test_addon)

    # May not succeed in case image used is already used by another un-related container.
    docker_manager.remove_addon_images(test_addon)

    # Assert that the container was removed
    containers = docker_client.containers.list(all=True)
    assert not any(container.id == created_container.id for container in containers)

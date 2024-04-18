import sys
from pathlib import Path

import docker
import pytest
import tests.container_utils as container_utils

current_dir = Path(__file__).parent
parent_dir = current_dir.parent
sys.path.append(str(parent_dir))

from services.addons_service import (  # noqa: E402
    ADDON_ENGINE_ID,
    ADDON_ID_LABEL,
    ADDON_MANAGER_LABEL,
    ADDON_SERVICE_NAME_LABEL,
    DEFAULT_NETWORK_NAME,
    DockerAddonRunner,
)


@pytest.fixture(scope="session")
def docker_client():
    return docker.from_env()


def test_run_addon_creates_container(docker_client):
    docker_manager = DockerAddonRunner()

    test_addon = container_utils.get_dummy_addon()
    result = docker_manager.run_addon(test_addon)
    assert result.get("new_containers") is not None
    assert len(result.get("new_containers")) == 1

    created_container = result.get("new_containers", [])[0]

    assert created_container is not None
    assert DEFAULT_NETWORK_NAME in created_container.attrs["NetworkSettings"]["Networks"]

    labels = created_container.labels
    assert labels[ADDON_ID_LABEL] == test_addon["_id"]
    assert labels[ADDON_MANAGER_LABEL] == ADDON_ENGINE_ID
    assert labels[ADDON_SERVICE_NAME_LABEL] == test_addon["services"][0]["service_name"]

    oak_containers = docker_manager._addons_monitor.get_oak_addon_containers()
    assert len(oak_containers) == 1
    assert oak_containers[0].id == created_container.id

    # Cleanup
    docker_manager.stop_addon(test_addon)

    # May not succeed in case image used is already used by another un-related container.
    docker_manager.remove_addon_images(test_addon)

    # Assert that the container was removed
    containers = docker_client.containers.list(all=True)
    assert not any(container.id == created_container.id for container in containers)

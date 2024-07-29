import sys
from pathlib import Path

import tests.container_utils as container_utils

current_dir = Path(__file__).parent
parent_dir = current_dir.parent
sys.path.append(str(parent_dir))

from services.monitor_service import (  # noqa: E402
    ADDONS_ID_LABEL,
    ADDONS_SERVICE_NAME_LABEL,
    DEFAULT_NETWORK,
    addons_monitor,
)


def test_run_addon_creates_container():
    test_addon = container_utils.get_dummy_addon()
    running_services, failed_services = addons_monitor.run_addon(test_addon)
    assert running_services is not None
    assert len(running_services) == 1

    created_container = running_services[0]

    assert created_container is not None
    assert DEFAULT_NETWORK in created_container.attrs["NetworkSettings"]["Networks"]

    labels = created_container.labels
    assert labels[ADDONS_ID_LABEL] == test_addon["_id"]
    assert labels[ADDONS_SERVICE_NAME_LABEL] == test_addon["services"][0]["service_name"]

    runner_engine = addons_monitor.get_addon_runner(test_addon)

    oak_containers = addons_monitor.get_oak_addon_containers(runner_engine)
    assert len(oak_containers) == len(test_addon["services"])
    assert oak_containers[0].id == created_container.id

    # Cleanup
    addons_monitor.stop_addon(test_addon)

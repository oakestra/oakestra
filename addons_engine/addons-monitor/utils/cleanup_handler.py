from addons_runner import runner_types
from services.monitor_service import addons_monitor


def handle_shutdown():
    runner_types = [runner_type.value for runner_type in runner_types.RunnerTypes]
    for runner_type in runner_types:
        containers = addons_monitor.get_oak_addon_containers()
        for container in containers:
            runner_type.stop_container(container)

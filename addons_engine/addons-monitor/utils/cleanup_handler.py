from addons_runner import runner_types
from services.monitor_service import addons_monitor


def handle_shutdown():
    runners = [runner_type.value for runner_type in runner_types.RunnerTypes]
    for runner_type in runners:
        runner_engine = runner_types.get_runner(runner_type)
        containers = addons_monitor.get_oak_addon_containers(runner_engine)
        for container in containers:
            runner_engine.stop_container(container)

from enum import Enum

from addons_runner.docker_runner import DockerRunner


class RunnerTypes(Enum):
    DOCKER = "docker"

    def __str__(self):
        return self.value


RUNNER_MAP = {RunnerTypes.DOCKER.value: DockerRunner()}


def get_runner(runner_type):
    return RUNNER_MAP.get(runner_type)

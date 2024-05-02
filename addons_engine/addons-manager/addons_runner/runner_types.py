from enum import Enum

from addons_runner.docker_runner import DockerRunner


class RunnerType(Enum):
    DOCKER = "docker"


RUNNER_MAP = {RunnerType.DOCKER.value: DockerRunner}


def get_runner(runner_type):
    # TODO the object is created every time, should be a singleton
    return RUNNER_MAP.get(runner_type, DockerRunner)

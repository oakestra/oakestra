from typing import Optional

from oakestra_utils.types.common import CustomEnum

# Legend
# - 🟨: Only mentioned in Schedulers.
# - 🟦: Explicitly mentioned in the Go NodeEngine and in python code.
# - 🟪: Only mentioned in the Go NodeEngine.
# - All others are only explicitly mentioned in non-scheduler Python code.


class Status(CustomEnum):
    pass


class LegacyStatus(Status):
    """TODO: Investigate how these legacy statuses are used and how to replace them gracefully.

    Note: These values had been ints before"""

    LEGACY_0 = "0"
    LEGACY_1 = "1"


class SchedulingStatus(Status):
    "Represents the state before a service gets deployed"

    pass


class NegativeSchedulingStatus(SchedulingStatus):
    TARGET_CLUSTER_NOT_FOUND = "TargetClusterNotFound"  # 🟨
    TARGET_CLUSTER_NOT_ACTIVE = "TargetClusterNotActive"  # 🟨
    TARGET_CLUSTER_NO_CAPACITY = "TargetClusterNoCapacity"  # 🟨

    NO_ACTIVE_CLUSTER_WITH_CAPACITY = "NoActiveClusterWithCapacity"  # 🟨

    NO_WORKER_CAPACITY = "NO_WORKER_CAPACITY"
    NO_QUALIFIED_WORKER_FOUND = "NO_QUALIFIED_WORKER_FOUND"  # 🟨
    NO_NODE_FOUND = "NO_NODE_FOUND"  # 🟨


class PositiveSchedulingStatus(SchedulingStatus):
    REQUESTED = "REQUESTED"
    CLUSTER_SCHEDULED = "CLUSTER_SCHEDULED"
    # The container is not yet created but the node is working on it.
    # E.g. pulling the image.
    NODE_SCHEDULED = "NODE_SCHEDULED"


class DeploymentStatus(Status):
    """Represents the state of a service during deployment,
    after it got scheduled."""

    CREATING = "CREATING"  # 🟪
    # The container is created (image downloaded, container metadata created, etc.)
    # but the (process) task is not yet running.
    CREATED = "CREATED"  # 🟪
    # The container is created and its task is running.
    RUNNING = "RUNNING"

    # The container task terminated with return code != 0 (failure).
    FAILED = "FAILED"  # 🟦

    # The container task terminated with return code  0 (success)
    # and is not a one-shot service - thus it should get restarted.
    DEAD = "DEAD"  # 🟦

    # The container task has terminated with return code 0 (success)
    # and the service was a one-shot - thus it should not be restarted.
    COMPLETED = "COMPLETED"  # 🟪

    # After termination (DEAD or FAILED) the container is undeployed from the system (success)
    UNDEPLOYED = "UNDEPLOYED"  # 🟪

    # Migration requested to a target node.
    # The target node still has to accept the mgiration request
    MIGRATION_REQUESTED = "MIGRATION_REQUESTED"  # 🟪

    # Migration request accepted by a target node.
    # The service is ready to be migrated.
    MIGRATION_ACCEPTED = "MIGRATION_ACCEPTED"  # 🟪

    # Service migration to target node in progress
    # Node FROM is sending the data to Node TO.
    MIGRATION_PROGRESS = "MIGRATION_IN_PROGRESS"  # 🟪

    # Migration completed successfully
    # The node FROM is slowly de-bouncing the traffinc and cleaning up the service.
    MIGRATION_DEBOUNCE = "MIGRATION_DEBOUNCING"  # 🟪


def convert_to_status(name: Optional[str]) -> Optional[Status]:
    """Converts a given string to its matching Status enum

    If None or "" is provided as input return None.

    This method is necessary because Status(name) does not work.
    """
    if not name:
        return None

    STATUS_CLASSES = [
        PositiveSchedulingStatus,
        NegativeSchedulingStatus,
        DeploymentStatus,
        LegacyStatus,
    ]
    for status_class in STATUS_CLASSES:
        if name in [enum.value for enum in status_class]:
            return status_class(name)
    raise ValueError(f"'{name}' does not match any Status Class.")

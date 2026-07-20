from oakestra_logging import get_logger
from resource_abstractor_client import candidate_operations, job_operations

logger = get_logger(__name__)


def find_cluster_of_job(job_id, instance_num=-1):
    logger.debug(
        "Finding cluster assigned to job",
        event_name="job.cluster.lookup",
        job_id=job_id,
        instance_number=instance_num,
    )

    job_obj = job_operations.get_job_by_id(job_id)
    if not job_obj:
        return None

    # TODO(ME): we can ask resource-abstractor to return the instance directly
    instances = job_obj.get("instance_list")
    if not instances:
        return None

    if instance_num == -1:
        return candidate_operations.get_candidate_by_id(instances[0]["cluster_id"])

    for instance in instances:
        if instance["instance_number"] == instance_num:
            return candidate_operations.get_candidate_by_id(instance["cluster_id"])

    return None

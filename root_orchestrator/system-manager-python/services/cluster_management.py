import logging

from resource_abstractor_client import cluster_operations, job_operations


def find_cluster_of_job(job_id, instance_num=-1):
    logging.log(logging.INFO, "Find job by Id and return cluster...")

    job_obj = job_operations.get_job_by_id(job_id)
    if not job_obj:
        return None

    # TODO: we can ask resource-abstractor to return the instance directly
    instances = job_obj.get("instance_list")
    if not instances:
        return None

    if instance_num == -1:
        return cluster_operations.get_cluster_by_id(instances[0]["cluster_id"])

    for instance in instances:
        if instance["instance_number"] == instance_num:
            return cluster_operations.get_cluster_by_id(instance["cluster_id"])

    return None

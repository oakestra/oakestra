import logging

from resource_abstractor_client import cluster_operations, job_operations


def find_cluster_of_job(job_id, instance_num):
    logging.log(logging.INFO, "Find job by Id and return cluster...")
    query = {}

    if instance_num != -1:
        query["instance_list"] = int(instance_num)

    job_obj = job_operations.get_job_by_id(job_id)

    if job_obj is None:
        return None

    instances = job_obj.get("instance_list")
    if instances and len(instances) > 0:
        instance = job_obj["instance_list"][0]
        return cluster_operations.get_resource_by_id(instance["cluster_id"])

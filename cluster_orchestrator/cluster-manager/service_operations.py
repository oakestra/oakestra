from cluster_scheduler_requests import scheduler_request_deploy
from mongodb_client import (
    mongo_create_new_job_instance,
    mongo_find_job_by_system_id,
    mongo_remove_job_instance,
)
from mqtt_client import mqtt_publish_edge_delete


def deploy_service(job, system_job_id, instance_number):
    job_obj = mongo_create_new_job_instance(job, system_job_id, int(instance_number))
    scheduler_request_deploy(job_obj, system_job_id, instance_number)
    return "ok"


def delete_service(system_job_id, instance_number, erase=True):
    # job_id is the system_job_id assigned by System Manager
    job = mongo_find_job_by_system_id(system_job_id)
    instance_list = job.get("instance_list")
    job_id = str(job.get("_id"))
    job.__setitem__("_id", job_id)
    for instance in instance_list:
        if int(instance["instance_number"]) == int(instance_number) or int(instance_number) == -1:
            node_id = instance.get("worker_id")
            if node_id is not None:
                mqtt_publish_edge_delete(
                    node_id,
                    job.get("job_name"),
                    instance["instance_number"],
                    job.get("virtualization", "docker"),
                )
            if erase:
                mongo_remove_job_instance(system_job_id, instance["instance_number"])
            if int(instance_number) != -1:
                break

    return "ok"

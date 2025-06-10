from ext_requests.cluster_scheduler_requests import scheduler_request_deploy
from clients.mongodb_client import (
    mongo_create_new_job_instance,
    mongo_find_job_by_system_id,
    mongo_remove_job_instance,
)
from clients.mqtt_client import mqtt_publish_edge_delete, mqtt_publish_edge_migrate
from utils.token_manager import generate_token


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


def service_migration(job, instance_number, target_node):
    """
    Send migration request to current job.
    :param job: The job object containing service details.
    :param target_node: The target node where the service will be migrated.
    """
    system_job_id = job.get("system_job_id")

    migration_request = {
        "job_id": system_job_id,
        "job_name": job.get("job_name"),
        "virtualization": job.get("virtualization", "docker"),
        "instance_number": int(instance_number),
        "target_node_id": target_node.get("_id"),
        "target_node_ip": target_node.get("node_info", {}).get("node_address"),
        "target_node_port": target_node.get("node_info", {}).get("node_port"),
        "migration_token": generate_token(64),
        "migration_scheme": "default",  # default migration scheme
    }

    # send migrationr equest to receiver worker node
    migration_request["type"] = "migration_receive"
    mqtt_publish_edge_migrate(
        target_node.get("_id"),
        migration_request
    )

    # send migration request to the active worker node
    migration_request["type"] = "migration_send"
    mqtt_publish_edge_migrate(
        job.get("instance_list")[0].get("worker_id"),
        migration_request
    )

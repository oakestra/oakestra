import threading

from ext_requests.cluster_requests import cluster_request_to_delete_job, cluster_request_to_deploy
from ext_requests.net_plugin_requests import (
    net_inform_instance_deploy,
    net_inform_instance_undeploy,
)
from ext_requests.scheduler_requests import scheduler_request_deploy
from resource_abstractor_client import app_operations, cluster_operations, job_operations


def update_job_status(job_id, status, status_detail, instances=None):
    job = job_operations.get_job_by_id(job_id)

    if job is None:
        return None

    if instances is not None:
        for instance in instances:
            job_operations.update_job_instance(job_id, instance["instance_number"], instance)

    return job_operations.update_job_status(job_id, status, status_detail)


def update_job_status_and_instances(
    job_id, status, next_instance_progressive_number, instance_list
):
    print("Updating Job Status and assigning a cluster for this job...")
    updated_job = job_operations.update_job(
        job_id,
        {
            "status": status,
            "next_instance_progressive_number": next_instance_progressive_number,
            "instance_list": instance_list,
        },
    )
    if updated_job is None:
        print(f"Updating job-{job_id} status failed")


def request_scale_up_instance(microserviceid, username):
    service = job_operations.get_job_by_id(microserviceid)
    print(service)
    application = app_operations.get_app_by_id(str(service["applicationID"]), username)
    if application is not None:
        if microserviceid in application["microservices"]:
            # Job status to scheduling REQUESTED
            update_job_status(microserviceid, "REQUESTED", "Waiting for scheduling decision")
            # Request scheduling
            threading.Thread(
                group=None,
                target=scheduler_request_deploy,
                args=(
                    service,
                    str(microserviceid),
                ),
            ).start()


def request_scale_down_instance(microserviceid, username, which_one=-1):
    """
    remove the instance <which_one> of a service.
    which_one default value is -1 which means "all instances"
    """
    service = job_operations.get_job_by_id(microserviceid)
    application = app_operations.get_app_by_id(str(service["applicationID"]), username)
    if application is not None:
        if microserviceid in application["microservices"]:
            service = job_operations.get_job_by_id(microserviceid)
            instances = service.get("instance_list")
            if len(instances) > 0:
                for instance in instances:
                    if which_one == instance["instance_number"] or which_one == -1:
                        net_inform_instance_undeploy(microserviceid, which_one)
                        cluster_request_to_delete_job(microserviceid, instance["instance_number"])
                        instances.remove(instance)
                update_job_status_and_instances(
                    microserviceid,
                    service["status"],
                    service["next_instance_progressive_number"],
                    instances,
                )


def instance_scale_up_scheduled_handler(job_id, cluster_id):
    job = job_operations.get_job_by_id(job_id)
    if job is None:
        return

    cluster = cluster_operations.get_resource_by_id(cluster_id)
    if cluster is None:
        return

    instance_number = job["next_instance_progressive_number"]
    instance_info = {
        "instance_number": instance_number,
        "cluster_id": cluster_id,
        "cluster_location": cluster.get("cluster_location", "location-unknown"),
    }
    instance_list = job["instance_list"]
    instance_list.append(instance_info)

    update_job_status_and_instances(
        job_id=job_id,
        status="CLUSTER_SCHEDULED",
        next_instance_progressive_number=instance_number + 1,
        instance_list=instance_list,
    )

    # inform network component
    net_inform_instance_deploy(str(job_id), instance_number, cluster_id)

    cluster_request_to_deploy(cluster_id, job_id, instance_number)

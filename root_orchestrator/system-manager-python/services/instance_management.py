import threading

from ext_requests.apps_db import (
    mongo_find_app_by_id,
    mongo_find_job_by_id,
    mongo_update_job_status,
    mongo_update_job_status_and_instances,
)
from ext_requests.cluster_db import mongo_find_cluster_by_id
from ext_requests.cluster_requests import cluster_request_to_delete_job, cluster_request_to_deploy
from ext_requests.net_plugin_requests import (
    net_inform_instance_deploy,
    net_inform_instance_undeploy,
)
from ext_requests.scheduler_requests import scheduler_request_deploy


def request_scale_up_instance(microserviceid, username):
    service = mongo_find_job_by_id(microserviceid)
    print(service)
    application = mongo_find_app_by_id(str(service["applicationID"]), username)
    if application is not None:
        if microserviceid in application["microservices"]:
            # Job status to scheduling REQUESTED
            mongo_update_job_status(microserviceid, "REQUESTED", "Waiting for scheduling decision")
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
    service = mongo_find_job_by_id(microserviceid)
    application = mongo_find_app_by_id(str(service["applicationID"]), username)
    if application is not None:
        if microserviceid in application["microservices"]:
            service = mongo_find_job_by_id(microserviceid)
            instances = service.get("instance_list")
            if len(instances) > 0:
                for instance in instances:
                    if which_one == instance["instance_number"] or which_one == -1:
                        net_inform_instance_undeploy(microserviceid, which_one)
                        cluster_request_to_delete_job(microserviceid, instance["instance_number"])
                        instances.remove(instance)
                mongo_update_job_status_and_instances(
                    microserviceid,
                    service["status"],
                    service["next_instance_progressive_number"],
                    instances,
                )


def instance_scale_up_scheduled_handler(job_id, cluster_id):
    job = mongo_find_job_by_id(job_id)
    if job is not None:
        cluster = mongo_find_cluster_by_id(cluster_id)
        instance_number = job["next_instance_progressive_number"]
        instance_info = {
            "instance_number": instance_number,
            "cluster_id": cluster_id,
            "cluster_location": cluster.get("cluster_location", "location-unknown"),
        }
        instance_list = job["instance_list"]
        instance_list.append(instance_info)

        mongo_update_job_status_and_instances(
            job_id=job_id,
            status="CLUSTER_SCHEDULED",
            next_instance_progressive_number=instance_number + 1,
            instance_list=instance_list,
        )

        # inform network component
        net_inform_instance_deploy(str(job_id), instance_number, cluster_id)

        cluster_request_to_deploy(cluster_id, job_id, instance_number)

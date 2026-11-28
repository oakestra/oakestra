from interfaces import clusters_interface, mongodb_requests
from operations import cluster_management


def deregister_interest(cluster_id, job_name):
    if cluster_id is None or job_name is None:
        return "Invalid input arguments", 400
    mongodb_requests.mongo_remove_cluster_job_interest(cluster_id, job_name)

    return "deregistered", 200


def notify_job_instance_undeployment(job_name, instancenum):
    _notify_clusters(clusters_interface.notify_undeployment, job_name, instancenum)


def notify_job_instance_deployment(job_name, instancenum):
    _notify_clusters(clusters_interface.notify_deployment, job_name, instancenum)


def _notify_clusters(handler, job_name, instancenum):
    clusters = mongodb_requests.mongo_get_cluster_interested_to_job(job_name)
    for cluster in clusters:
        result = handler(cluster["cluster_address"], cluster["cluster_port"], job_name, instancenum)
        if result != 200:
            cluster_management.set_cluster_status(
                cluster["cluster_id"], cluster_management.CLUSTER_STATUS_ERROR
            )
        else:
            cluster_management.set_cluster_status(
                cluster["cluster_id"], cluster_management.CLUSTER_STATUS_ACTIVE
            )

from interfaces.mongodb_requests import mongo_get_cluster_interested_to_job, mongo_register_cluster_job_interest, \
    mongo_remove_cluster_job_interest
from interfaces import clusters_interface


def deregister_interest(cluster_id, job_name):
    if cluster_id is None or job_name is None:
        return "Invalid input arguments", 400
    mongo_remove_cluster_job_interest(cluster_id, job_name)


def notify_job_instance_undeployment(job_name, instancenum):
    clusters = mongo_get_cluster_interested_to_job(job_name)
    for cluster in clusters:
        clusters_interface.notify_undeployment(
            cluster["cluster_info"]["cluster_ip"],
            cluster["cluster_info"]["cluster_port"],
            job_name,
            instancenum
        )

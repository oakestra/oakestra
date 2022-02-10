from network.subnetwork_management import *
from interfaces.mongodb_requests import *
from network import tablequery, routes_interests


def deploy_request(sys_job_id=None, replicas=None, cluster_id=None):
    if sys_job_id is None or replicas is None or cluster_id is None:
        return "Invalid input parameters", 400
    mongo_update_job_status_and_instances_by_system_job_id(
        system_job_id=sys_job_id,
        instance_list=_prepare_instance_list(replicas, cluster_id)
    )
    return "Instance info added", 200


def update_instance_local_addresses(instances=None, job_id=None):
    if instances is None or job_id is None:
        return "Invalid input parameters", 400
    mongo_update_job_net_status(
        job_id=job_id,
        instances=instances
    )
    return "Status updated", 200


def undeploy_request(sys_job_id=None, instance=None):
    if sys_job_id is None or instance is None:
        return "Invalid input parameters", 400
    if (mongo_update_clean_one_instance(
            system_job_id=sys_job_id,
            instance=instance)):
        job = mongo_find_job_by_systemid(sys_job_id)
        routes_interests.notify_job_instance_undeployment(job.get("job_name"), instance)
        return "Instance info cleared", 200
    return "Instance not found", 400


def get_service_instances(name=None, ip=None, cluster_ip=None):
    if cluster_ip is None:
        return "Invalid address", 400
    cluster = mongo_get_cluster_by_ip(cluster_ip)

    if cluster is None:
        return "Invalid cluster address, is the cluster registered?", 400

    job = tablequery.service_resolution(name=name, ip=ip)

    if job is None:
        return "Job not found", 404

    # route interest registration for this route
    mongo_register_cluster_job_interest(cluster.get("cluster_id"), job.get("job_name"))

    return {
                "job_name": job.get("job_name"),
                "system_job_id": job.get("system_job_id"),
                "instance_list": job.get("instance_list"),
                "service_ip_list": job.get("service_ip_list")
           }, 200


def _prepare_instance_list(replicas, cluster_id):
    instance_list = []
    for i in range(replicas):
        instance_info = {
            'instance_number': i,  # number generation must be changed when scale up and down ops are implemented
            'instance_ip': new_instance_ip(),
            'cluster_id': str(cluster_id),
        }
        instance_list.append(instance_info)

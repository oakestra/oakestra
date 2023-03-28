from network.routes_interests import notify_job_instance_deployment
from network.subnetwork_management import *
from interfaces import mongodb_requests
from network import tablequery, routes_interests


def deploy_request(sys_job_id=None, instance_number=None, cluster_id=None):
    if sys_job_id is None or instance_number is None or cluster_id is None:
        return "Invalid input parameters", 400
    mongodb_requests.mongo_create_job_instance(
        system_job_id=sys_job_id,
        instance=_prepare_instance_dict(instance_number, cluster_id)
    )
    return "Instance info added", 200


def update_instance_local_addresses(job_id=None, instances=None):
    if instances is None or job_id is None:
        return "Invalid input parameters", 400
    for instance in instances:
        assert instance.get("instance_number") is not None
        assert instance.get("namespace_ip") is not None
        assert instance.get("host_ip") is not None
        assert instance.get("host_port") is not None

    job = mongodb_requests.mongo_update_job_net_status(
        job_id=job_id,
        instances=instances
    )

    if job is None:
        return "Job not found", 404

    for instance in instances:
        notify_job_instance_deployment(job["job_name"], instance.get("instance_number"))

    return "Status updated", 200


def undeploy_request(sys_job_id=None, instance_number=None):
    if sys_job_id is None or instance_number is None:
        return "Invalid input parameters", 400
    if (mongodb_requests.mongo_update_clean_one_instance(
            system_job_id=sys_job_id,
            instance_number=instance_number)):
        job = mongodb_requests.mongo_find_job_by_systemid(sys_job_id)
        routes_interests.notify_job_instance_undeployment(job.get("job_name"), instance_number)
        return "Instance info cleared", 200
    return "Instance not found", 400


def get_service_instances(name=None, ip=None, cluster_ip=None):
    if cluster_ip is None:
        return "Invalid address", 400
    cluster = mongodb_requests.mongo_get_cluster_by_ip(cluster_ip)

    if cluster is None:
        return "Invalid cluster address, is the cluster registered?", 400

    job = tablequery.service_resolution(name=name, ip=ip)

    if job is None:
        return "Job not found", 404

    # route interest registration for this route
    mongodb_requests.mongo_register_cluster_job_interest(cluster.get("cluster_id"), job.get("job_name"))

    if job.get("_id") is not None:
        job["_id"] = str(job["_id"])

    return job, 200


def _prepare_instance_dict(isntance_number, cluster_id):

    return {
        'instance_number': isntance_number,
        'instance_ip': new_instance_ip(),
        'instance_ip_v6': new_instance_ip_v6(),
        'cluster_id': str(cluster_id),
    }

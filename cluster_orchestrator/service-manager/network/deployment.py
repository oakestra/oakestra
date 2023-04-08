from interfaces import mongodb_requests
from interfaces.root_service_manager_requests import *


def deployment_status_report(appname, status, NsIp, NsIPv6, node_id, instance_number, host_ip, host_port):
    # Update mongo job
    mongodb_requests.mongo_update_job_deployed(appname, status, NsIp, NsIPv6, node_id, instance_number, host_ip, host_port)
    job = mongodb_requests.mongo_find_job_by_name(appname)
    # Notify System manager
    system_manager_notify_deployment_status(job, node_id)

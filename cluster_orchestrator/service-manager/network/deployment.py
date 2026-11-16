from interfaces import mongodb_requests, mqtt_client
from interfaces.root_service_manager_requests import *


def deployment_status_report(
    appname, status, NsIp, NsIPv6, node_id, instance_number, host_ip, host_port
):
    # Update mongo job
    job = mongodb_requests.mongo_update_job_deployed(
        appname, status, NsIp, NsIPv6, node_id, instance_number, host_ip, host_port
    )
    if job is None:
        raise FileNotFoundError
    # Notify System manager
    system_manager_notify_deployment_status(job, node_id)


def deployment_address_update(appname, node_id, instance_number, host_ip, host_port):
    # Update mongo
    job = mongodb_requests.mongo_update_job_address(
        appname, node_id, instance_number, host_ip, host_port
    )
    if job is None:
        raise FileNotFoundError

    # Notify System manager
    system_manager_notify_deployment_status(job, node_id)

    # Notify all interested worker nodes of the address change
    mqtt_client.mqtt_notify_service_change(appname, type="DEPLOYMENT")

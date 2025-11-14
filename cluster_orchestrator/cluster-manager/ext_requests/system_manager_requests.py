import os
import threading
import traceback

import requests
import services.service_operations as service_operations
from clients.mongodb_client import (
    mongo_find_job_by_system_id,
    mongo_aggregate_node_information,
    mongo_get_services_with_failed_instanes,
)
from clients.my_prometheus_client import prometheus_set_metrics
from oakestra_utils.types.statuses import (
    DeploymentStatus,
    NegativeSchedulingStatus,
    convert_to_status,
)
from clients.mqtt_client import mqtt_publish_edge_deploy

from ext_requests.cluster_scheduler_requests import scheduler_request_deploy

SYSTEM_MANAGER_ADDR = (
    "http://" + os.environ.get("SYSTEM_MANAGER_URL") + ":" + os.environ.get("SYSTEM_MANAGER_PORT")
)


def send_aggregated_info_to_sm(my_id, time_interval):
    try:
        data = mongo_aggregate_node_information(time_interval)
        threading.Thread(group=None, target=send_aggregated_info, args=(my_id, data)).start()
        prometheus_set_metrics(data)
    except Exception as e:
        print(e)
        traceback.print_exc()


def re_deploy_dead_services_routine():
    re_deploy_triggers = [
        DeploymentStatus.FAILED,
        DeploymentStatus.DEAD,
        NegativeSchedulingStatus.NO_WORKER_CAPACITY,
    ]

    try:
        services = mongo_get_services_with_failed_instanes()
        if services is not None:
            for service in services:
                for instance in service.get("instance_list", []):
                    status = convert_to_status(instance.get("status"))
                    # we trigger a re-deploy only if the instance is not currently deployed
                    #  or its status is in the re_deploy_triggers list
                    if status in re_deploy_triggers or instance.get("node") is None:
                        print("FAILED INSTANCE, ATTEMPTING RE-DEPLOY")
                        threading.Thread(
                            group=None,
                            target=trigger_undeploy_and_re_deploy,
                            args=(service, instance),
                        ).start()
                    else:
                        # if the instance is still deployed but in UNKNOWN status
                        # we refresh its deployment
                        print("REFRESHING DEPLOYMENT")
                        threading.Thread(
                            group=None,
                            target=refresh_deployment,
                            args=(service, instance),
                        ).start()
                            
    except Exception as e:
        print(e)
        traceback.print_exc()


def send_aggregated_info(my_id, data):
    print("Sending aggregated information to System Manager.")
    try:
        requests.post(SYSTEM_MANAGER_ADDR + "/api/information/" + str(my_id), json=data)
    except requests.exceptions.RequestException:
        print("Calling System Manager /api/information not successful.")


def trigger_undeploy_and_re_deploy(service, instance):
    try:
        service_operations.delete_service(
            service.get("system_job_id"), instance.get("instance_number"), erase=False
        )
        scheduler_request_deploy(
            service,
            str(service.get("system_job_id")),
            str(instance.get("instance_number")),
        )
    except Exception as e:
        print(e)


# Tries to re-deploy the service in the same machine as before.
# The node will discard the re-deploy if the instance is still running.
def refresh_deployment(service, instance):
    try:
        node_id = service.get("node").get("_id")
        job = mongo_find_job_by_system_id(service.get("system_job_id"))
        mqtt_publish_edge_deploy(node_id, job, instance)
    except Exception as e:
        print(e)


def cloud_request_incr_node(my_id):
    print("reporting to cloud about new worker node...")
    request_addr = SYSTEM_MANAGER_ADDR + "/api/cluster/" + str(my_id) + "/incr_node"
    print(request_addr)
    try:
        requests.get(request_addr)
    except requests.exceptions.RequestException:
        print("Calling System Manager /api/cluster/../incr_node not successful.")

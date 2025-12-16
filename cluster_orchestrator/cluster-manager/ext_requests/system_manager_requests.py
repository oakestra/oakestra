import os
import threading
import traceback
import requests
from clients.my_prometheus_client import prometheus_set_metrics
from oakestra_utils.types.statuses import (
    DeploymentStatus,
    NegativeSchedulingStatus,
    convert_to_status,
)
from ext_requests.scheduler_requests import scheduler_request_deploy
from clients import worker_management, job_management

SYSTEM_MANAGER_ADDR = (
    "http://" + os.environ.get("SYSTEM_MANAGER_URL") + ":" + os.environ.get("SYSTEM_MANAGER_PORT")
)

def send_aggregated_info_to_sm(my_id, time_interval):
    try:
        data = worker_management.aggregate_info(time_interval)
        data.update({"jobs": job_management.aggregate_info(time_interval)})
        print("sending aggregated info to system manager: ", data)
        threading.Thread(group=None, target=send_aggregated_info, args=(my_id, data)).start()
        prometheus_set_metrics(data)
    except Exception as e:
        print(e)
        traceback.print_exc()


def re_deploy_dead_jobs_routine():
    re_deploy_triggers = [
        DeploymentStatus.FAILED,
        DeploymentStatus.DEAD,
        NegativeSchedulingStatus.NO_WORKER_CAPACITY,
    ]
    try:
        jobs = job_management.get_jobs_with_failed_instances()
        if jobs is not None:
            for job in jobs:
                for instance in job.get("instance_list", []):
                    if convert_to_status(instance.get("status")) in re_deploy_triggers:
                        print("FAILED INSTANCE, ATTEMPTING RE-DEPLOY")
                        threading.Thread(
                            group=None,
                            target=trigger_undeploy_and_re_deploy,
                            args=(job, instance),
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
        job_management.delete_job_instance(
            service.get("_id"), instance.get("instance_number"), erase=False
        )
        scheduler_request_deploy(service, instance.get("instance_number"))
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

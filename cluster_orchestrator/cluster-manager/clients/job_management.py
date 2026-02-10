import logging
from datetime import datetime, timedelta

from ext_requests.scheduler_requests import scheduler_request_deploy
from oakestra_utils.types.statuses import (
    DeploymentStatus,
    LegacyStatus,
    NegativeSchedulingStatus,
    PositiveSchedulingStatus,
    convert_to_status,
)
from resource_abstractor_client import candidate_operations, job_operations

logger = logging.getLogger("cluster_manager")


def mark_inactive_as_failed(time_interval):
    cutoff = (datetime.now() - timedelta(seconds=time_interval)).timestamp()
    query = {
        "instance_list": {
            "$elemMatch": {
                "$or": [
                    {"last_modified_timestamp": {"$lt": cutoff}},
                ]
            }
        }
    }

    jobs = job_operations.get_jobs(**query)
    if jobs is None:
        return

    for job in jobs:
        failed_instances = []

        for instance in job["instance_list"]:
            job_status = convert_to_status(instance.get("status", None)) or LegacyStatus.LEGACY_0

            timestamp = instance.get("last_modified_timestamp", datetime.now().timestamp())

            if (
                timestamp < cutoff
                and job_status not in PositiveSchedulingStatus
                and job_status != DeploymentStatus.COMPLETED
            ):
                update_instance(
                    job.get("_id"),
                    instance.get("instance_number"),
                    {
                        "status": DeploymentStatus.FAILED.value,
                        "status_detail": "No suitable worker found",
                    },
                )
                failed_instances.append(instance.get("instance_number"))

        if failed_instances:
            job_operations.update_job(
                job.get("_id"),
                {
                    "status": DeploymentStatus.FAILED.value,
                    "status_detail": "Failed instance(s): "
                    + ", ".join(str(x) for x in failed_instances),
                },
            )

    return


def aggregate_info(time_interval):
    mark_inactive_as_failed(time_interval)
    jobs = job_operations.get_jobs() or []

    return [
        {
            "_id": job.get("_id"),
            "job_name": job.get("job_name"),
            "status": job.get("status", int(LegacyStatus.LEGACY_1.value)),
            "instance_list": job.get("instance_list"),
        }
        for job in jobs
    ]


def create_new_job_instance(job: dict, instance_number: int):
    job_id = job.get("_id")
    if job_id is None or job_operations.get_job_by_id(job_id) is None:
        return job_operations.create_job(job)
    return job_operations.update_job_instance(job_id, instance_number, job)


def update_deployed_instance_worker(job_name, instance_number, status, public_ip, worker_id):
    jobs = job_operations.get_jobs(job_name=job_name)
    if not jobs:
        return

    job_id = jobs[0].get("_id")

    update_status(job_id, int(instance_number), status)
    update_instance(job_id, int(instance_number), {"publicip": public_ip})


def update_status(job_id, instance_number, status, status_detail=None):
    if status == DeploymentStatus.CREATED.value:
        return

    job = job_operations.get_job_by_id(job_id)
    if job is None:
        return

    # don't update job to running
    if status != DeploymentStatus.RUNNING.value:
        job["status"] = status

    if job.get("instance_list") is not None:
        for instance in job.get("instance_list"):
            if instance["instance_number"] == instance_number:
                instance["status"] = status
                if status_detail is not None:
                    instance["status_detail"] = status_detail

    job_operations.update_job(job_id, job)


def update_deployed_instance_job(job_name, instance_number, service, worker_id):
    jobs = job_operations.get_jobs(job_name=job_name)
    if not jobs:
        return None

    job_id = jobs[0].get("_id")
    update_status(
        job_id,
        int(instance_number),
        DeploymentStatus.RUNNING.value,
        service.get("status_detail", None),
    )
    data = {
        "cpu_percent": service.get("cpu_percent"),
        "memory_percent": service.get("memory_percent"),
        "disk": service.get("disk"),
        "logs": service.get("logs", ""),
    }
    return update_instance(job_id, int(instance_number), data)


def update_instance_node(job_id, instance_number, worker_id):
    node = candidate_operations.get_candidate_by_id(worker_id)
    data = {
        "host_ip": node.get("ip"),
        "host_port": 50011 if node.get("port", "") == "" else node.get("port"),
        "worker_id": worker_id,
    }
    return update_instance(job_id, instance_number, data)


def update_instance(job_id, instance_number, data):
    job = job_operations.get_job_by_id(job_id)
    if job is None:
        return None

    updated_instance = None

    # Filter none value
    data = {k: v for k, v in data.items() if v is not None}
    data["last_modified_timestamp"] = datetime.now().timestamp()

    if job.get("instance_list", None) is None:
        data["instance_number"] = instance_number
        job["instance_list"] = [data]
        updated_instance = data
    else:
        found = False
        for instance in job["instance_list"]:
            if instance["instance_number"] == instance_number:
                instance.update(data)
                updated_instance = instance
                found = True
                break

        if not found:
            data["instance_number"] = instance_number
            job["instance_list"].append(data)
            updated_instance = data

    return job_operations.update_job_instance(job_id, instance_number, updated_instance)


def deploy_job(job, instance_number):
    job_obj = create_new_job_instance(job, int(instance_number))
    scheduler_request_deploy(job_obj, int(instance_number))
    return "ok"


def get_jobs_with_failed_instances():
    query = {
        "$or": [
            {"instance_list.status": DeploymentStatus.FAILED.value},
            {"instance_list.status": DeploymentStatus.DEAD.value},
            {"instance_list.status": NegativeSchedulingStatus.NO_WORKER_CAPACITY.value},
        ]
    }
    return job_operations.get_jobs(**query)


def delete_job_instance(job_id: int, instance_number: int, erase: bool = True):
    from clients.mqtt_client import mqtt_publish_edge_delete

    # send instance undeployment to node
    job = job_operations.get_job_by_id(job_id)
    instance_list = job.get("instance_list")

    deleted_job = 0
    for instance in instance_list:
        if int(instance["instance_number"]) == instance_number or instance_number == -1:
            logger.info(f"Deleting instance {instance['instance_number']} of job {job_id}")
            deleted_job += 1
            worker_id = instance.get("worker_id", None)

            if worker_id is not None:
                mqtt_publish_edge_delete(
                    worker_id,
                    job.get("job_name"),
                    instance["instance_number"],
                    job.get("virtualization", "docker"),
                )

            # remove from db if erase is true
            if erase:
                job_operations.delete_job_instance(job_id, instance["instance_number"])
                logger.info(
                    f"Deleted instance {instance['instance_number']} of job {job_id} from DB"
                )

    if len(instance_list) <= deleted_job:
        job_operations.delete_job(job_id)
        logger.info(f"Deleted job {job_id} from DB as all instances were removed")
        return {}

    job = job_operations.get_job_by_id(job_id)
    return job

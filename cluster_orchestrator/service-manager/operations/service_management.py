from interfaces import mqtt_client, root_service_manager_requests, mongodb_requests
from operations.instances_management import instance_updates
import logging
import traceback
import copy

from interfaces.mongodb_requests import mongo_remove_job, mongo_update_job_instance


def create_service(job_name):
    if job_name is None:
        return "Invalid argument", 400

    # table query the root to get the instances
    try:
        job = root_service_manager_requests.cloud_table_query_service_name(job_name)
        mongodb_requests.mongo_insert_job(copy.deepcopy(job))
        for instance in job.get('instance_list'):
            mongo_update_job_instance(job_name, instance)
    except Exception as e:
        logging.error('Incoming Request /api/net/deployment failed service_resolution')
        logging.debug(traceback.format_exc())
        print(traceback.format_exc())
        return "Service resolution failed", 500

    return "job stored succesfully", 200


def remove_service(job_name):
    if job_name is None:
        return "Invalid argument", 400

    # get job from local db
    # for each instance, if any, ask undeploy

    # table query the root to get the instances
    job = mongodb_requests.mongo_find_job_by_name(job_name)
    if job is not None:
        try:
            for instance in job['instance_list']:
                instance_updates(job_name, instance['instance_number'], 'UNDEPLOYMENT')
        except Exception as e:
            logging.error('Incoming Request DELETE /api/net/deployment/jobname failed request instance undeployment')
            logging.debug(traceback.format_exc())
    mongo_remove_job(job_name)
    return "job removed succesfully", 200

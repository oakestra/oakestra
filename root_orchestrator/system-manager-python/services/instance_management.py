import threading

from ext_requests.apps_db import mongo_find_app_by_id, mongo_find_job_by_id, mongo_find_cluster_of_job, \
    mongo_update_job_status
from ext_requests.cluster_requests import cluster_request_to_delete_job
from ext_requests.net_plugin_requests import net_inform_instance_undeploy, net_inform_service_deploy
from ext_requests.scheduler_requests import scheduler_request_deploy


def scale_up_instance(microserviceid, username):
    service = mongo_find_job_by_id(microserviceid)
    application = mongo_find_app_by_id(service["applicationID"], username)
    if application is not None:
        if microserviceid in application["microservices"]:
            # Inform network plugin about the deployment
            threading.Thread(group=None, target=net_inform_service_deploy, args=(service, str(microserviceid),)).start()
            # Job status to scheduling REQUESTED
            mongo_update_job_status(microserviceid, 'REQUESTED')
            # Request scheduling
            threading.Thread(group=None, target=scheduler_request_deploy, args=(service, str(microserviceid),)).start()


def scale_down_instance(microserviceid, username, how_many=-1):
    """
    scale down <how_many> the instances of a service.
    how_many default value is -1 which means "all instances"
    """
    service = mongo_find_job_by_id(microserviceid)
    application = mongo_find_app_by_id(service["applicationID"], username)
    if application is not None:
        if microserviceid in application["microservices"]:
            service = mongo_find_job_by_id(microserviceid)
            instances = service.get("instance_list")
            if instances is not None:
                for instance in instances:
                    if how_many != 0:
                        how_many = how_many - 1
                        net_inform_instance_undeploy(microserviceid, instance['instance_number'])
                        cluster_obj = mongo_find_cluster_of_job(microserviceid)
                        cluster_request_to_delete_job(cluster_obj, microserviceid, instance['instance_number'])


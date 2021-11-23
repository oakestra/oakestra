from requests.mongodb_requests import *
from requests.root_service_manager_requests import *

def deployment_status_report(job_id,status,NsIp,node_id):
    # Update mongo job
    mongo_update_job_deployed(job_id, status, NsIp, node_id) #TODO does not work, update thejob accordingly
    job = mongo_find_job_by_id(job_id)
    app.logger.debug(job)
    # Notify System manager
    system_manager_notify_deployment_status(job, node_id)

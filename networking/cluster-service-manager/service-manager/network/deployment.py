from interfaces.mongodb_requests import *
from interfaces.root_service_manager_requests import *

def deployment_status_report(appname,status,NsIp,node_id,instance_number,host_ip,host_port):
    # Update mongo job
    mongo_update_job_deployed(appname, status, NsIp, node_id, instance_number,host_ip,host_port)
    job = mongo_find_job_by_name(appname)
    # Notify System manager
    system_manager_notify_deployment_status(job, node_id)

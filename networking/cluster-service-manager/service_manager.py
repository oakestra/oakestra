import os
from flask import Flask, request
import eventlet

from cluster_balancer import service_resolution, service_resolution_ip
from mongodb_client import mongo_init, mongo_find_node_by_id_and_update_subnetwork, mongo_update_job_deployed, \
    mongo_find_job_by_id
from net_logging import configure_logging
from root_service_manager_requests import root_service_manager_get_subnet, system_manager_notify_deployment_status

MY_PORT = os.environ.get('CLUSTER_SERVICE_MANAGER_PORT')

my_logger = configure_logging()
app = Flask(__name__)
mongo_init(app)


# ............. Node network management Endpoints ............#
# ............................................................#

@app.route('/api/node/ip/newsubnet/<node_id>', methods=['GET'])
def table_query_resolution_by_ip(node_id):
    """
    Get all the instances of a job given a Service IP in 172_30_x_y notation
    returns {
                addr: string
            }
    """
    app.logger.info("Incoming Request /api/node/ip/newsubnet/" + str(node_id))
    addr = root_service_manager_get_subnet()
    mongo_find_node_by_id_and_update_subnetwork(node_id, addr)
    return {'addr': addr}


# TODO: node status update

# ............. Job network management Endpoints ............#
# ...........................................................#

@app.route('/api/job/deployment/status', methods=['POST'])
def deploy_task():
    """
       Update job status
       receives {
                   job_id: string
                   status: string
                   nsip: string
                   node_id: string
                }
    """
    app.logger.info('Incoming Request /api/job/deployment/status')
    req_json = request.json
    app.logger.debug(req_json)
    mongo_update_job_deployed(req_json["job_id"], req_json["status"], req_json["nsip"], req_json["node_id"])
    job = mongo_find_job_by_id(req_json["job_id"])
    system_manager_notify_deployment_status(job, req_json["node_id"])

    return "ok"

@app.route('/api/job/<job_name>/instances', methods=['GET'])
def table_query_resolution_by_jobname(job_name):
    """
    Get all the instances of a job given the complete name
    """
    service_ip = job_name.replace("_", ".")
    app.logger.info("Incoming Request /api/job/" + str(job_name) + "/instances")
    return {'instance_list': service_resolution(job_name)}


@app.route('/api/job/ip/<service_ip>/instances', methods=['GET'])
def table_query_resolution_by_ip(service_ip):
    """
    Get all the instances of a job given a Service IP in 172_30_x_y notation
    returns {
                app_name: string
                instance_list: [
                    {
                        instance_number: int
                        namespace_ip: string
                        host_ip: string
                        host_port: string
                        service_ip: [
                            {
                                IpType: string
                                Address: string
                            }
                        ]
                    }
                ]
    """
    service_ip = service_ip.replace("_", ".")
    app.logger.info("Incoming Request /api/job/ip/" + str(service_ip) + "/instances")
    name, instances = service_resolution_ip(service_ip)
    return {'app_name': name, 'instance_list': instances}

# TODO: job migration
# TODO: job undeployment
# TODO: job scale up


if __name__ == '__main__':
    eventlet.wsgi.server(eventlet.listen(('0.0.0.0', int(MY_PORT))), app, log=my_logger)

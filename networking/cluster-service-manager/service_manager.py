import os
from flask import Flask, request
import eventlet

from tablequery.interests import register_interest_sname
from tablequery.resolution import service_resolution, service_resolution_ip
from requests.mongodb_requests import mongo_init, mongo_find_node_by_id_and_update_subnetwork, \
    mongo_update_job_deployed, \
    mongo_find_job_by_id, mongo_insert_job
from net_logging import configure_logging
from requests.root_service_manager_requests import root_service_manager_get_subnet, \
    system_manager_notify_deployment_status

MY_PORT = os.environ.get('CLUSTER_SERVICE_MANAGER_PORT')

my_logger = configure_logging()
app = Flask(__name__)
mongo_init(app)


# ............. Node network management Endpoints ............#
# ............................................................#

@app.route('/api/net/subnet/<node_id>', methods=['GET'])
def table_query_resolution_by_ip(node_id):
    """
    Endpoint used to require a new node's subnetwork
    """
    app.logger.info("Incoming Request /api/node/ip/newsubnet/" + str(node_id))
    addr = root_service_manager_get_subnet()
    mongo_find_node_by_id_and_update_subnetwork(node_id, addr)
    return {'addr': addr}


# TODO: node status report

# ............. Deployment Endpoints ............#
# ...........................................................#

@app.route('/api/net/deployment', methods=['POST'])
def deploy_task_status():
    """
       Deployment of a new service instance
       receives {
                   job_id: string
                   job: {}object
                }
    """

    app.logger.info('Incoming Request /api/net/deployment')
    req_json = request.json
    app.logger.debug(req_json)
    job_name = req_json['job']['job_name']

    # table query the root to get the instances
    instances, siplist = service_resolution(job_name)

    if siplist is not None:
        # register interest
        register_interest_sname(job_name, None)
        # add DB entry
        mongo_insert_job(job_name, req_json, siplist, instances)
        return "ok"

    app.logger.error('Incoming Request /api/net/deployment failed service_resolution')
    return 500


@app.route('/api/net/service/net_deploy_status', methods=['POST'])
def deploy_task_status():
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
    deployment_status_report(req_json["job_id"], req_json["status"], req_json["nsip"], req_json["node_id"])

    return "ok"


# ................. Table Query Endpoints ...................#
# ...........................................................#

@app.route('/api/net/job/<job_name>/instances', methods=['GET'])
def table_query_resolution_by_jobname(job_name):
    """
    Get all the instances of a job given the complete name
    """
    service_ip = job_name.replace("_", ".")
    app.logger.info("Incoming Request /api/job/" + str(job_name) + "/instances")
    return {'instance_list': service_resolution(job_name)}


@app.route('/api/net/job/ip/<service_ip>/instances', methods=['GET'])
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

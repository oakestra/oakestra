import os
from flask import Flask, request
from flask_socketio import SocketIO, emit

from cluster_balancer import service_resolution, service_resolution_ip
from mongodb_client import mongo_init
from cm_logging import configure_logging


MY_PORT = os.environ.get('SERVICE_MANAGER_PORT')

MY_CHOSEN_CLUSTER_NAME = os.environ.get('CLUSTER_NAME')
MY_CLUSTER_LOCATION = os.environ.get('CLUSTER_LOCATION')
SYSTEM_MANAGER_ADDR = 'http://' + os.environ.get('SERVICE_MANAGER_URL') + ':' + os.environ.get('SERVICEMANAGER_PORT')

my_logger = configure_logging()
app = Flask(__name__)
mongo_init(app)

# ............. Network management Endpoint ............#
# ......................................................#

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

import os
from flask import Flask, request
from flask_socketio import SocketIO, emit
import eventlet

from interfaces.mqtt_client import mqtt_init
from network.tablequery.interests import register_interest_sname
from network.tablequery.resolution import service_resolution, service_resolution_ip
from interfaces.mongodb_requests import mongo_init, mongo_insert_job
from net_logging import configure_logging

MY_PORT = os.environ.get('MY_PORT') or 10200

my_logger = configure_logging()
app = Flask(__name__)
socketio = SocketIO(app, async_mode='eventlet', logger=True, engineio_logger=True, cors_allowed_origins='*')
app.logger.addHandler(my_logger)
mongo_init(app)
mqtt_init(app)

# ............. Deployment Endpoints ............#
# ...........................................................#

@app.route('/api/net/deployment', methods=['POST'])
def deploy_task_status():
    """
       Deployment of a new service instance
       receives {
                   job_id: string
                   data: {}object
                }
    """

    app.logger.info('Incoming Request /api/net/deployment')
    req_json = request.json
    app.logger.debug(req_json)
    job_name = req_json['data']['job_name']

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


# ................. Table Query Endpoints ...................#
# ..................SOON TO BE DEPRECATED....................#

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
    import eventlet

    eventlet.wsgi.server(eventlet.listen(('0.0.0.0', int(MY_PORT))), app, log=my_logger)

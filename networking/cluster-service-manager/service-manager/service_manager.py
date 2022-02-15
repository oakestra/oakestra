import os
from flask import Flask, request
from flask_socketio import SocketIO

from interfaces.mqtt_client import mqtt_init
from net_logging import configure_logging
from interfaces.mongodb_requests import mongo_init
from interfaces.root_service_manager_requests import cloud_register_cluster
from operations.instances_management import instance_deployment, instance_updates

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
def deploy_task():
    """
       Deployment of a new service instance
       receives {
                   system_job_id: string
                   data: {}object
                }
    """

    app.logger.info('Incoming Request /api/net/deployment')
    req_json = request.json
    app.logger.debug(req_json)
    job_name = req_json['data']['job_name']

    return instance_deployment(job_name, req_json)


@app.route('/api/net/job/update', methods=['POST'])
def task_update():
    """
           Updates regarding a new service instance
           receives {
                "job_name": job_name,
                "instance_number": instance_number,
                "type": "DEPLOYMENT" or "UNDEPLOYMENT"
            }
    """
    app.logger.info('Incoming Request /api/net/job/update')
    req_json = request.json
    app.logger.debug(req_json)
    return instance_updates(
        job_name=req_json.get('job_name'),
        instancenum=req_json.get('instance_number'),
        type=req_json.get('type')
    )


# TODO: job migration
# TODO: job scale up

if __name__ == '__main__':
    import eventlet
    if cloud_register_cluster(MY_PORT):
        eventlet.wsgi.server(eventlet.listen(('0.0.0.0', int(MY_PORT))), app, log=my_logger)

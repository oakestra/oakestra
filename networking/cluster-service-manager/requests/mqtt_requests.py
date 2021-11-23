import os
import re
import json
from network.deployment import *
from datetime import datetime
from flask_mqtt import Mqtt
from tablequery.resolution import *
from tablequery.interests import *

mqtt = None
app = None


def mqtt_init(flask_app):
    global mqtt
    global app
    app = flask_app

    app.config['MQTT_BROKER_URL'] = os.environ.get('MQTT_BROKER_URL')
    app.config['MQTT_BROKER_PORT'] = int(os.environ.get('MQTT_BROKER_PORT'))
    app.config['MQTT_REFRESH_TIME'] = 1.0  # refresh time in seconds
    mqtt = Mqtt(app)

    @mqtt.on_connect()
    def handle_connect(client, userdata, flags, rc):
        app.logger.info("MQTT - Connected to MQTT Broker")
        mqtt.subscribe('nodes/+/net/+')

    @mqtt.on_log()
    def handle_logging(client, userdata, level, buf):
        if level == 'MQTT_LOG_ERR':
            app.logger.info('Error: {}'.format(buf))

    @mqtt.on_message()
    def handle_mqtt_message(client, userdata, message):
        data = dict(
            topic=message.topic,
            payload=message.payload.decode()
        )
        app.logger.info('MQTT - Received from worker: ')
        app.logger.info(data)

        topic = data['topic']

        re_job_deployment_topic = re.search("^nodes/.*/net/service/deployed$", topic)
        re_job_undeployment_topic = re.search("^nodes/.*/net/service/undeployed$", topic)
        re_job_tablequery_topic = re.search("^nodes/.*/net/tablequery/request", topic)

        topic_split = topic.split('/')
        client_id = topic_split[1]
        payload = json.loads(data['payload'])

        if re_job_deployment_topic is not None:
            app.logger.debug('JOB-DEPLOYMENT-UPDATE')
            _deployment_handler(client_id, payload)
        if re_job_undeployment_topic is not None:
            app.logger.debug('JOB-UNDEPLOYMENT-UPDATE')
            _undeployment_handler(client_id, payload)
        if re_job_tablequery_topic is not None:
            app.logger.debug('JOB-TABLEQUERY-REQUEST')
            _tablequery_handler(client_id, payload)


def _deployment_handler(client_id, payload):
    job_id = payload.get('job_id')
    status = payload.get('status')
    nsIp = payload.get('ns_ip')
    deployment_status_report(job_id, status, nsIp, client_id)


def _undeployment_handler(client_id, payload):
    # TODO
    pass


def _tablequery_handler(client_id, payload):
    sname = payload.get('sname')
    sip = payload.get('sip')

    result = {}
    instances = {}

    # resolve the query and register interest
    if sip is not None:
        sname, instances = service_resolution_ip(sip)
    elif sname is not None:
        instances = service_resolution(sname)

    register_interest_sname(sname, client_id)
    result = {'app_name': sname, 'instance_list': instances}
    mqtt_publish_tablequery_result(client_id, result)


def mqtt_publish_tablequery_result(client_id, result):
    topic = 'nodes/' + client_id + '/net/tablequery/result'
    mqtt.publish(topic, json.dumps(result))

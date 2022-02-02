import re
from network.deployment import *
from network.tablequery.resolution import *
from network.tablequery.interests import *
from flask_mqtt import Mqtt

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
        mqtt.subscribe('nodes/+/net/#')

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

        re_job_deployment_topic = re.search("^nodes/.*/net/service/deployed", topic)
        re_job_undeployment_topic = re.search("^nodes/.*/net/service/undeployed", topic)
        re_job_tablequery_topic = re.search("^nodes/.*/net/tablequery/request", topic)
        re_job_subnet_topic = re.search("^nodes/.*/net/subnet", topic)

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
        if re_job_subnet_topic is not None:
            app.logger.debug('JOB-SUBNET-REQUEST')
            _subnet_handler(client_id, payload)


def _deployment_handler(client_id, payload):
    appname = payload.get('appname')
    status = payload.get('status')
    nsIp = payload.get('nsip')
    instance_number = payload.get('instance_number')
    host_ip = payload.get('host_ip')
    host_port = payload.get('host_port')
    deployment_status_report(appname, status, nsIp, client_id,instance_number,host_ip,host_port)


def _undeployment_handler(client_id, payload):
    # TODO
    pass


def _tablequery_handler(client_id, payload):
    sname = payload.get('sname')
    sip = payload.get('sip')

    instances = []
    siplist = []

    # resolve the query and register interest
    if sip is not None and sip != "":
        instances, siplist = service_resolution_ip(sip)
    elif sname is not None and sname != "":
        instances, siplist = service_resolution(sname)

    register_interest_sname(sname, client_id)
    result = {'app_name': sname, 'instance_list': format_instance_response(instances,siplist)}
    mqtt_publish_tablequery_result(client_id, result)


def _subnet_handler(client_id, payload):
    method = payload.get('METHOD')
    if method == 'GET':
        # associate new subnetwork to the node
        addr = root_service_manager_get_subnet()
        mongo_find_node_by_id_and_update_subnetwork(client_id, addr)
        mqtt_publish_subnetwork_result(client_id, {"address": addr})
    elif method == 'DELETE':
        # remove subnetwork from node
        pass


def mqtt_publish_tablequery_result(client_id, result):
    topic = 'nodes/' + client_id + '/net/tablequery/result'
    mqtt.publish(topic, json.dumps(result))


def mqtt_publish_subnetwork_result(client_id, result):
    topic = 'nodes/' + client_id + '/net/subnetwork/result'
    mqtt.publish(topic, json.dumps(result))

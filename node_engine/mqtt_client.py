import os
import socket

import numpy as np
from ast import literal_eval

from requests import get
from flask_mqtt import Mqtt
import json
import re
import time

from cpu_mem import get_cpu_memory, get_memory
from dockerclient import start_container, stop_container
from mirageosclient import run_unikernel_mirageos
from coordinates import get_coordinates
from vivaldi_coordinate import VivaldiCoordinate

mqtt = None
app = None
rtt = None
vivaldi_coordinate = None

def mqtt_init(flask_app, mqtt_port=1883, my_id=None):
    global mqtt
    global app
    global req
    global vivaldi_coordinate

    app = flask_app
    vivaldi_coordinate = VivaldiCoordinate(3)
    app.config['MQTT_BROKER_URL'] = os.environ.get('CLUSTER_MANAGER_IP')
    app.config['MQTT_BROKER_PORT'] = int(mqtt_port)
    app.config['MQTT_REFRESH_TIME'] = 3.0  # refresh time in seconds
    mqtt = Mqtt(app)
    app.logger.info('initialized mqtt')
    mqtt.subscribe('nodes/' + my_id + '/control/+')
    mqtt.subscribe('nodes/' + my_id + '/ack')
    # Subscribe to the ping channel. If a worker receives a message via that channel it should ping the IP contained in the message
    mqtt.subscribe('nodes/' + my_id + '/ping')
    mqtt.subscribe('nodes/' + my_id + '/vivaldi')

    @mqtt.on_message()
    def handle_mqtt_message(client, userdata, message):
        data = dict(
            topic=message.topic,
            payload=json.loads(message.payload.decode())
        )
        app.logger.info(data)

        topic = data.get('topic')
        # if topic starts with nodes and ends with controls
        re_nodes_topic_control_deploy = re.search("^nodes/" + my_id + "/control/deploy$", topic)
        re_nodes_topic_control_delete = re.search("^nodes/" + my_id + "/control/delete$", topic)
        re_nodes_topic_ack = re.search("^nodes/" + my_id + "/ack$", topic)
        re_nodes_topic_ping = re.search("^nodes/" + my_id + "/ping$", topic)
        re_nodes_topic_vivaldi = re.search("^nodes/" + my_id + "/vivaldi", topic)
        if re_nodes_topic_ack is not None:
            payload = data.get('payload')
            req = payload.get('request_time')
            resp = time.time()
            global rtt
            rtt = (resp - req) * 1000 # in ms
            app.logger.info('CO - Worker RTT: {}'.format(rtt))
        else:
            payload = data.get('payload')
            image_technology = payload.get('image_runtime')
            image_url = payload.get('image')
            job_name = payload.get('job_name')
            port = payload.get('port')

        # If the node received a message via the ping channel it should ping the IP contained in the received message
        if re_nodes_topic_ping is not None:
            app.logger.info("MQTT - Received PING command")
            payload = data.get('payload')
            target_ip = payload.get('target_ip')
            avg_rtt = ping(target_ip)
            app.logger.info(f"Average RTT to user is {avg_rtt}")

        if re_nodes_topic_vivaldi is not None:
            app.logger.info("MQTT - Received Vivaldi command")
            payload = data.get('payload')
            vivaldi_info = payload.get('vivaldi_info')
            app.logger.info(f"Received vivaldi infos: {vivaldi_info}")
            ip_vivaldi_dict = {}
            for info in vivaldi_info:
                ip = info[0]
                vector = info[1]
                height = info[2]
                error = info[3]
                remote_vivaldi = VivaldiCoordinate(5)
                remote_vivaldi.vector = np.array(vector)
                remote_vivaldi.height = float(height)
                remote_vivaldi.error = float(error)
                ip_vivaldi_dict[ip] = remote_vivaldi

            # Ping received IPs in parallel
            statistics = parallel_ping(ip_vivaldi_dict.keys())
            for ip, rtt in statistics.items():
                vivaldi_coordinate.update(rtt, ip_vivaldi_dict[ip])

        if re_nodes_topic_control_deploy is not None:
            app.logger.info("MQTT - Received .../control/deploy command")
            if image_technology == 'docker':
                start_container(image=image_url, name=job_name, port=port)
            if image_technology == 'mirage':
                commands = payload.get('commands')
                run_unikernel_mirageos(image_url, job_name, job_name, commands)
        elif re_nodes_topic_control_delete is not None:
            app.logger.info('MQTT - Received .../control/delete command')
            if image_technology == 'docker':
                stop_container(job_name)


def publish_cpu_mem(my_id):
    app.logger.info('Publishing CPU+Memory usage... my ID: {0}'.format(my_id))
    cpu_used, free_cores, memory_used, free_memory_in_MB = get_cpu_memory()
    mem_value = get_memory()
    topic = 'nodes/' + my_id + '/information'
    lat, long = get_coordinates()
    app.logger.info(f"RTT: {rtt}")
    app.logger.info(f"Vivaldi: {vivaldi_coordinate.vector} {vivaldi_coordinate.height}")
    # TODO: how do we know whether nodes are within a network (i.e. don't ping the public ip but the local ips of the nodes)
    #  or distributed accross several networks (i.e. ping punblic ip)?
    #  -> for the latency test in FSOC lab just send the local ip
    # ip = get('https://api.ipify.org').text
    ip = socket.gethostbyname(socket.gethostname())
    mqtt.publish(topic, json.dumps({'cpu': cpu_used, 'free_cores': free_cores,
                                    'memory': memory_used, 'memory_free_in_MB': free_memory_in_MB,
                                    'lat': lat, 'long': long, 'request_time': time.time(),
                                    'rtt': rtt,
                                    'vivaldi_vector': vivaldi_coordinate.vector.tolist(),
                                    'vivaldi_height': vivaldi_coordinate.height,
                                    'vivaldi_error': vivaldi_coordinate.error, 'public_ip': ip,
                                    'netem_delay': get_netem_delay()}))


def ping(target_ip):
    import platform
    import subprocess
    # Parameter for number of packets differs between the operating systems
    param = '-n' if platform.system().lower() == 'windows' else '-c'
    # TODO: how many packets should we send?
    command = ['ping', param, '3', target_ip]
    response = subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.PIPE).communicate()
    regex_pattern = "rtt min/avg/max/mdev = (\d+.\d+)/(\d+.\d+)/(\d+.\d+)/(\d+.\d+)"
    # times = min,avg,max,mdev
    # TODO: use min rtt. for some reason first ping in docker is twice as expected latency
    times = re.findall(regex_pattern, str(response))[0]
    avg_rtt = times[0]
    app.logger.info(f"Ping {target_ip} RTT:{avg_rtt}ms")

    return avg_rtt

def parallel_ping(target_ips):
    import io
    import os
    import sys
    from itertools import islice
    from subprocess import Popen
    import re
    ON_POSIX = 'posix' in sys.builtin_module_names
    # Create a pipi to get data
    input_fd, output_fd = os.pipe()
    # start several subprocesses
    processes = [Popen(['ping', '-c', '3', ip], stdout=output_fd, close_fds=ON_POSIX) for ip in target_ips]
    os.close(output_fd)
    statistics = {}
    ip_pattern = "\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}"
    rtt_pattern = "rtt min/avg/max/mdev = (\d+.\d+)/(\d+.\d+)/(\d+.\d+)/(\d+.\d+)"

    with io.open(input_fd, 'r', buffering=1) as file:
        for line in file:
            if 'ping statistics' in line:
                # Find target ip
                ip_match = re.search(ip_pattern, line)
                # Find RTTs
                statistic = ''.join(islice(file, 2))
                statistic_match = re.findall(rtt_pattern, statistic)
                if len(statistic_match) != 0 and ip_match is not None:
                    ip = ip_match[0]
                    stat = statistic_match[0]
                    min_rtt = float(stat[0])
                    avg_rtt = float(stat[1])
                    statistics[ip] = min_rtt

    for p in processes:
        p.wait()

    return statistics

def get_netem_delay():
    import subprocess
    command = ['tc', 'qdisc']
    response = subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.PIPE).communicate()
    resp = str(response[0])
    delay_idx = resp.index('delay')
    netem_delay = resp[delay_idx + 6 : -5]
    return netem_delay

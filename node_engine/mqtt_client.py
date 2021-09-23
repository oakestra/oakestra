import ipaddress
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
vivaldi_coordinate = None
public_ip = None
private_ip = None
router_rtt = None
node_info = {}


def mqtt_init(flask_app, mqtt_port=1883, my_id=None):
    global mqtt
    global app
    global req
    global vivaldi_coordinate
    global public_ip
    global private_ip
    global router_rtt

    app = flask_app
    vivaldi_coordinate = VivaldiCoordinate(3)
    public_ip, private_ip, router_rtt = get_ip_info()
    app.config['MQTT_BROKER_URL'] = os.environ.get('MQTT_IP')
    app.config['MQTT_BROKER_PORT'] = int(mqtt_port)
    app.config['MQTT_REFRESH_TIME'] = 3.0  # refresh time in seconds
    mqtt = Mqtt(app)
    app.logger.info('initialized mqtt')
    mqtt.subscribe('nodes/' + my_id + '/control/+')
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
        re_nodes_topic_ping = re.search("^nodes/" + my_id + "/ping$", topic)
        re_nodes_topic_vivaldi = re.search("^nodes/" + my_id + "/vivaldi", topic)

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
            # Dict has target IP as key and a tuple consisting of the remote VivaldiCoordinate and the router_rtt if required
            ip_vivaldi_dict = {}
            for info in vivaldi_info:
                # vivaldi_info = (public_ip, private_ip, router_rtt, vector, height, error)
                remote_public_ip = info[0]
                remote_private_ip = info[1]
                remote_router_rtt = float(info[2])
                remote_vector = info[3]
                remote_height = info[4]
                remote_error = info[5]
                remote_vivaldi = VivaldiCoordinate(5)
                remote_vivaldi.vector = np.array(remote_vector)
                remote_vivaldi.height = float(remote_height)
                remote_vivaldi.error = float(remote_error)
                # this node is in same network as remote and both behind router -> same public ip and private ip not null -> ping private ip
                if public_ip == remote_public_ip:
                    ip_vivaldi_dict[remote_private_ip] = (remote_vivaldi, None)
                # This node and the remote node are not within the same network
                if public_ip != remote_public_ip:
                    ip_vivaldi_dict[remote_public_ip] = (remote_vivaldi, remote_router_rtt)

            # Ping received IPs in parallel
            statistics = parallel_ping(ip_vivaldi_dict.keys())
            for ip, rtt in statistics.items():
                viv, r_rtt = ip_vivaldi_dict[ip] # TODO: Naming!
                if r_rtt is not None:
                    rtt += r_rtt
                vivaldi_coordinate.update(rtt, viv)

        if re_nodes_topic_control_deploy is not None:
            app.logger.info("MQTT - Received .../control/deploy command")
            address = None
            if image_technology == 'docker':
                address = start_container(job=payload)
            if image_technology == 'mirage':
                commands = payload.get('commands')
                run_unikernel_mirageos(image_url, job_name, job_name, commands)
            if address is not None:
                publish_deploy_status(node_info.id, payload.get('_id'), 'DEPLOYED', address)
            else:
                publish_deploy_status(node_info.id, payload.get('_id'), 'FAILED', '')
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
    # If ip address of this node is private, the node has to ping the network router such that this RTT can be added
    # to nodes pinging this network. Otherwise the Vivaldi network coordinates would update themself with respect to
    # the router and not the nodes within the network

    is_netem_configured = os.environ.get('IS_NETEM_CONFIGURED') == 'TRUE'
    mqtt.publish(topic, json.dumps({'cpu': cpu_used, 'free_cores': free_cores,
                                    'memory': memory_used, 'memory_free_in_MB': free_memory_in_MB,
                                    'lat': lat, 'long': long, 'request_time': time.time(),
                                    'vivaldi_vector': vivaldi_coordinate.vector.tolist(),
                                    'vivaldi_height': vivaldi_coordinate.height,
                                    'vivaldi_error': vivaldi_coordinate.error,
                                    'public_ip': public_ip, 'private_ip': private_ip, 'router_rtt': router_rtt,
                                    'netem_delay': get_netem_delay(is_netem_configured)}))


def get_ip_info():
    #ip = socket.gethostbyname(socket.gethostname())
    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    s.connect(("8.8.8.8", 80))
    ip = s.getsockname()[0]
    s.close()
    # TODO: can the IP address obtained via socket even be a public ip?
    if ipaddress.ip_address(ip).is_private:
        public_ip = get('https://api.ipify.org').text
        router_rtt = ping(public_ip)
        private_ip = ip
        app.logger.info(f"is_private: public={public_ip}, private={private_ip}, router_rtt={router_rtt}")
    else:
        public_ip = ip
        private_ip = None
        router_rtt = None
        app.logger.info(f"is_public: public={public_ip}, private={private_ip}, router_rtt={router_rtt}")

    return public_ip, private_ip, router_rtt

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

def get_netem_delay(is_netem_configured):
    if is_netem_configured:
        import subprocess
        command = ['tc', 'qdisc']
        response = subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.PIPE).communicate()
        resp = str(response[0])
        delay_idx = resp.index('delay')
        netem_delay = resp[delay_idx + 6 : -5]
        return netem_delay
    else:
        return "0.0"


def publish_deploy_status(my_id, job_id, status, ns_ip):
    app.logger.info('Publishing Deployment status... my ID: {0}'.format(my_id))
    topic = 'nodes/' + my_id + '/job'
    mqtt.publish(topic, json.dumps({'job_id': job_id, 'status': status,
                                    'ns_ip': ns_ip}))

import json

import requests
import time
from collections import Counter

from flask_mqtt import Mqtt
from pyshark import LiveCapture
from shapely.geometry import Polygon, Point

# import mqtt_client
from NetworkCoordinateSystem.network_measurements import parallel_ping
from dockerclient import is_running

from node_engine import app as flask_app

# import logging
# flask_app.logger.disabled = True
# log = logging.getLogger('werkzeug')
# log.disabled = True
TYPE = 'type'
# TODO: Temporary area mapping until we know how we select area in SLA
MUNICH = Polygon([[48.24819, 11.50406], [48.22807, 11.63521], [48.18093, 11.69083], [48.1369, 11.72242],
                  [48.07689, 11.68534], [48.06221, 11.50818], [48.13008, 11.38871], [48.15757, 11.36124],
                  [48.20107, 11.39077]])
GARCHING = Polygon([[48.26122, 11.59742], [48.27013, 11.68445], [48.21720, 11.65063], [48.23013, 11.59862]])
PUBLIC_IP = "46.244.221.241"

AREAS = {
    "munich": MUNICH,
    "garching": GARCHING
}
# mqtt = None
from Monitoring import celeryapp
# from flask import current_app
# REDIS_ADDR = os.environ.get('REDIS_ADDR')
# REDIS_ADDR="redis://:workerRedis@localhost:6380"
# TODO: do i really need to keep the resutls (i.e. result backend) or can i ommit it for resources sake
# celeryapp = Celery(__name__, backend=REDIS_ADDR, broker=REDIS_ADDR)

def monitor_docker_container(payload, container_id):
    constraints = payload.get('constraints')
    job = payload.get('job')
    port = job['port']
    port_out = port.split(":")[0]
    print("Interating constraints")
    for constraint in constraints:
        constraint_type = constraint.get(TYPE)

        print(f"Constraint type={constraint_type}")
        if constraint_type == 'geo':
            monitor_geo_constraint.delay(container_id, port_out, constraint)
        elif constraint_type == 'latency':
            monitor_latency_constraint.delay(container_id, port_out, job, constraint)
        else:
            raise AttributeError(f"no such constraint type {constraint_type}")


@celeryapp.task
def monitor_latency_constraint(container_id, port, job, constraint):
    global mqtt
    # mqtt_client.mqtt_init(flask_app, 10003, my_id=None)
    print(f"Celery worker: START TASK LATENCY")
    flask_app.config['MQTT_BROKER_URL'] = PUBLIC_IP
    flask_app.config['MQTT_BROKER_PORT'] = 10003
    flask_app.config['MQTT_REFRESH_TIME'] = 3.0  # refresh time in seconds
    mqtt = Mqtt(flask_app)
    print('Celery worker: initialized mqtt')
    coord_ip_mapping = {}
    violation_ctr = Counter()
    area = constraint['area'].lower()
    while is_running(container_id):
        # Get GeoLite2 location for src IP addresses
        coord_ip_mapping = {**coord_ip_mapping, **listen_to_port(container_id, port)}
        # Check whether any connected IP is located within the area specified in the SLA
        for coord, ips in coord_ip_mapping.items():
            lat, long = coord.split(",")
            request_location = Point(float(lat), float(long))
            if area in AREAS:
                # Check if request_location is within constraint area
                if AREAS[area].contains(request_location) or request_location.touches(AREAS[area]):
                    print(f"Request comes from {area}")
                    # In this case we have to monitor the latency to the ips in this area
                    violation_ctr = measure_latency_violations(job, ips, constraint, violation_ctr)
            else:
                raise AttributeError(f"Area '{area}' is not supported.")
    print(f"{container_id}: {coord_ip_mapping}")
    print(f"Celery worker: END TASK LATENCY")

@celeryapp.task
def monitor_geo_constraint(container_id, port, constraint):
    return "GEO"

def listen_to_port(container, port):
    interface = 'any'
    # worker_public_ip = os.environ.get("WORKER_PUBLIC_IP")
    worker_public_ip = PUBLIC_IP
    display_filter = f'http.host == {worker_public_ip}:{port} and http'
    capture = LiveCapture(interface=interface, display_filter=display_filter)
    print(f"Listening on {interface} filtering by {display_filter}")
    # capture = capture.sniff(timeout=8) TODO: assignment needed for test until figured out how to change state of mock
    capture.sniff(timeout=8)
    capture_size = len(capture)
    location_ip_mapping = {}
    if capture_size >= 1:
        for i in range(capture_size):
            packet = capture[i]
            try:
                localtime = time.asctime(time.localtime(time.time()))
                src_addr = packet.ip.src
                dst_addr = packet.ip.dst
                host = packet['http'].host
                #dst_addr, dst_port = str(host).split(":")
                request_uri = packet['http'].request_uri

                # Geolocation
                # request_address = 'http://' + os.environ.get('SYSTEM_MANAGER_IP') + ':' + str(os.environ.get('SYSTEM_MANAGER_PORT'))
                request_address = 'http://192.168.178.33:10000/geolocation'
                # TODO: Build dict for already queued ips to avoid redundant API calls
                print("Query geolocation from System Manager")
                response = requests.get(f"{request_address}/{src_addr}")
                location = json.loads(response.text)
                print(location)
                # location = query_geolocation_for_ip(src_addr)
                location_key = f"{location['lat']},{location['long']}"
                if (location_key in location_ip_mapping and src_addr not in location_ip_mapping[location_key]) or location_key not in location_ip_mapping:
                    location_ip_mapping.setdefault(location_key, []).append(src_addr)
                print(f"{container}: {localtime} {src_addr} -> {dst_addr} URI: {request_uri} Host: {host}")
            except AttributeError as e:
                # ignore other packets
                pass
    print(f"done sniffing on {container}")
    return location_ip_mapping


def measure_latency_violations(job, ips, constraint, violation_ctr):
    threshold = constraint['threshold']
    # {ip1: <min rtt>, ip2: <min rtt>, ...}
    statistics = parallel_ping(ips)
    print(f"Ping stats: {statistics}")
    # Measurement tolerance
    tol = 0.2
    # Allowed violations
    allowed_violations = 1
    violations = {}

    # check if any ping exceeds threshold
    for ip, rtt in statistics.items():
        if rtt > threshold + (threshold * tol):
            violation_ctr[ip] += 1
            if violation_ctr[ip] >= allowed_violations:
                violations[ip] = statistics[ip]

    print(f"Violations: {violations} Violation Counter: {violation_ctr}")
    if len(violations) >= 1:
        # Send alarm to cluster orchestrator
        my_id = job['instance_list'][0]['worker_id']
        violated_job = job.copy()
        violated_job['constraints'] = constraint
        publish_sla_alarm(my_id, violated_job, violations)

    return violation_ctr

def publish_sla_alarm(my_id, violated_job, violations):
    # app.logger.info(f"Publishing SLA violation alarm... my ID: {my_id}")
    print(f"Publishing SLA violation alarm... my ID: {my_id}\n")
    topic = f"nodes/{my_id}/alarm"
    # violations = {<violating ip>: <violating rtt>,...}
    mqtt.publish(topic, json.dumps({'job': violated_job, 'violations': violations}))
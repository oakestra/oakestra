import os
import requests
import numpy as np
from collections import Counter
import json
from pyshark import LiveCapture
from shapely.geometry import Polygon, Point
import geopy.distance
import ipaddress
from app.blueprints.network_measurement import network_measurement
from app.blueprints.node_engine import dockerclient, cpu_mem, coordinates
from paho.mqtt.client import Client

from app.extensions.celery import celery as celeryapp
from app.models.vivaldi_coordinate import VivaldiCoordinate

TYPE = 'type'
MUNICH = Polygon([[48.24819, 11.50406], [48.22807, 11.63521], [48.18093, 11.69083], [48.1369, 11.72242],
                  [48.07689, 11.68534], [48.06221, 11.50818], [48.13008, 11.38871], [48.15757, 11.36124],
                  [48.20107, 11.39077]])
GARCHING = Polygon([[48.26122, 11.59742], [48.27013, 11.68445], [48.21720, 11.65063], [48.23013, 11.59862]])
GERMANY = Polygon([[53.69039499952727, 7.19896078851138],[54.83252457729241, 9.022691264644848],[54.258998141984534, 13.17552331270781],
                   [51.13131284515075, 14.625718631079001], [50.29656693243139, 12.120835808437851], [48.69842804429054, 13.768785033859663],
                   [47.613790266506015, 12.208726433793686], [47.732154490129666, 7.638413915290528], [48.973220613985035, 8.209702980103422],
                   [49.5040219286023, 6.320054534953079]])
AREAS = {
    "munich": MUNICH,
    "garching": GARCHING,
    "germany": GERMANY # Used for testing different latency measures from requests within geremany
}

PUBLIC_IP = os.environ.get('MYIP')
ALLOWED_VIOLATIONS = 3

# Note: use of redis causes warning "redis-py works best with hiredis. Please consider installing
# Open issue: https://github.com/redis/redis-py/issues/1725
# celeryapp = Celery(__name__, backend=REDIS_ADDR, broker=REDIS_ADDR)
geolocation_cache = {}

# from app.extensions.celery import celery as celeryapp
# from app import celery as celeryapp


def publish_sla_alarm(node_id, alarm_type, violated_job, ip_rtt_stats=None):
    # logger.info(f"Publishing SLA violation alarm... my ID: {my_id}")
    print(f"Publishing SLA violation alarm of type '{alarm_type}'... my ID: {node_id}\n")
    # topic = f"nodes/{my_id}/alarm"
    topic = f"nodes/{node_id}/alarm"
    mqtt = Client()
    mqtt.connect(os.environ.get("CLUSTER_MANAGER_IP"), 10003, 10)
    # ip_rtt_stats = {<violating ip>: <violating rtt>,...} only required for latency constraint violations
    mqtt.publish(topic, json.dumps({"job": violated_job, "ip_rtt_stats": ip_rtt_stats}))
    mqtt.disconnect()
    # mqtt.loop_start()


@celeryapp.task
def monitor_docker_container(job, container_id, container_port, node_id):
    print(f"Start monitoring container {node_id} {container_id}")
    # Initialize violation counters
    s2u_latency_violations_ctr = Counter()
    s2u_geo_violations_ctr = Counter()
    s2s_latency_violations_ctr = Counter()
    s2s_geo_violations_ctr = Counter()
    memory_violations_ctr = Counter()
    cpu_violations_ctr = Counter()
    coord_ip_mapping = {}
    while dockerclient.is_running(container_id):
        try:
            # Check memory and CPU usage
            cpu_used, free_cores, mem_used, free_memory_in_MB = cpu_mem.get_cpu_memory()
            print("###################### Memory Constraint ######################")
            memory_violations_ctr = check_memory_constraint(job, node_id, container_id, mem_used, memory_violations_ctr)
            print("###################### CPU Constraint ######################")
            cpu_violations_ctr = check_cpu_constraint(job, node_id, cpu_used, cpu_violations_ctr)
            print("###################### Service-2-User Constraints ######################")
            s2u_geo_violations_ctr, s2u_latency_violations_ctr = check_service_to_user_constraints(job, node_id, container_id, container_port, coord_ip_mapping, s2u_geo_violations_ctr, s2u_latency_violations_ctr)
            print("###################### Service-2-Service Constraints ######################")
            s2s_geo_violations_ctr, s2s_latency_violations_ctr = check_service_to_service_constraints(job, node_id, s2s_geo_violations_ctr, s2s_latency_violations_ctr)
            # Remove entries exceeding the allowed number of violations from monitoring
            remove_violations_from_counter(cpu_violations_ctr)
            remove_violations_from_counter(memory_violations_ctr)
            remove_violations_from_counter(s2u_geo_violations_ctr)
            remove_violations_from_counter(s2u_latency_violations_ctr)
            remove_violations_from_counter(s2s_geo_violations_ctr)
            remove_violations_from_counter(s2s_latency_violations_ctr)
        except KeyError:
            print(f"Container was interrupted.")

    print(f"Done monitoring container {container_id}")


def check_memory_constraint(job, node_id, container_id, mem_used, counter):
    # if memory usage lower than memory defined in job AND memory usage >95%
    required_memory_in_mb = job["memory"]
    print("Check current memory usage.")
    try:
        used_memory_in_mb = dockerclient.get_memory_usage_in_mb(container_id)
    except json.decoder.JSONDecodeError:
        return counter

    print(f"Container: required: {required_memory_in_mb} used: {used_memory_in_mb}")
    print(f"System: used: {mem_used}")
    print(f"Counter: {counter}")
    if used_memory_in_mb > required_memory_in_mb and mem_used >= 0.95:
        counter["mem"] += 1
    if counter["mem"] >= ALLOWED_VIOLATIONS:
        print(f"Exceeded violation threshold of {ALLOWED_VIOLATIONS}. Trigger SLA alarm.")
        # Send alarm to cluster orchestrator
        publish_sla_alarm(node_id, "memory", job)

    return counter


def check_cpu_constraint(job, node_id, cpu_used, counter):
    print("Check CPU usage")
    print(f"used: {cpu_used}% ")
    print(f"Counter: {counter} ")
    if cpu_used >= 95:
        counter["cpu"] += 1
    if counter["cpu"] >= ALLOWED_VIOLATIONS:
        print(f"Exceeded violation threshold of {ALLOWED_VIOLATIONS}. Trigger SLA alarm.")
        # Send alarm to cluster orchestrator
        publish_sla_alarm(node_id, "cpu", job)

    return counter


def check_service_to_service_constraints(job, node_id, geo_violations_ctr, latency_violations_ctr):
    connectivity = job.get("connectivity")
    for conn in connectivity:
        con_constraints = conn.get("con_constraints")
        target_worker_info = conn.get("target_worker_info")
        for constraint in con_constraints:
            con_type = constraint.get("type")
            threshold = constraint.get("threshold")
            if con_type == "geo":
                print("###################### S2S GEO ######################")
                geo_violations_ctr = check_s2s_geo_constraint(node_id, target_worker_info, threshold, geo_violations_ctr, job)
            elif con_type == 'latency':
                print("###################### S2S LATENCY ######################")
                latency_violations_ctr = check_s2s_latency_constraint(node_id, target_worker_info, threshold, latency_violations_ctr, job)
            else:
                raise AttributeError(f"no such constraint type {con_type}")

    return geo_violations_ctr, latency_violations_ctr

def check_s2s_geo_constraint(node_id, target_worker_info, threshold, counter, job):
    target_worker_id = target_worker_info.get("id")
    target_worker_coords = target_worker_info.get("loc")
    target_worker_lat = target_worker_coords[0]
    target_worker_long = target_worker_coords[1]
    print(f"Check geo constraint to worker {target_worker_id} with threshold {threshold}km")
    # Allowed violations
    worker_lat, worker_long = coordinates.get_coordinates()
    distance = geopy.distance.distance([worker_lat, worker_long], [target_worker_lat, target_worker_long]).km
    print(f"Distance between node ({worker_lat},{worker_long}) and target worker ({target_worker_coords}): {distance}km")
    if distance > threshold:
        print("Distance larger than threshold. Increment violation counter.")
        counter[f"{target_worker_coords[0]},{target_worker_id[1]}"] += 1
    if counter[f"{target_worker_coords[0]},{target_worker_coords[1]}"] >= ALLOWED_VIOLATIONS:
        print(f"Exceeded violation threshold of {ALLOWED_VIOLATIONS}. Trigger SLA alarm.")
        # Send alarm to cluster orchestrator
        publish_sla_alarm(node_id, "s2s-geo", job)

    return counter

def check_s2s_latency_constraint(node_id, target_worker_info, threshold, counter, job):
    # Request required Vivaldi info from cluster manager
    cluster_ip = os.environ.get("CLUSTER_MANAGER_IP")
    target_worker_id = target_worker_info.get("id")
    request_address = f"http://{cluster_ip}:10000/api/vivaldi-info"
    response = requests.post(request_address, json=json.dumps([node_id, target_worker_id]))
    node_id_vivaldi_mapping = response.json()
    # Create Vivaldi Coordinate of this worker
    worker_viv = create_vivaldi_coord(node_id_vivaldi_mapping[node_id])
    # Create Vivaldi Coordinate of target worker
    target_viv = create_vivaldi_coord(node_id_vivaldi_mapping[target_worker_id])
    # Check if distance is below threshold
    dist = worker_viv.distance(target_viv)
    tol = 0.2
    print(f"Latency to target: {dist} Threshold: {threshold + (threshold * tol)}")
    if dist >= threshold + (threshold * tol):
        print("Latency larger than threshold. Increment violation counter.")
        counter[target_worker_id] += 1
    if counter[target_worker_id] >= ALLOWED_VIOLATIONS:
        print(f"Exceeded violation threshold of {ALLOWED_VIOLATIONS}. Trigger SLA alarm.")
        # Send alarm to cluster orchestrator
        publish_sla_alarm(node_id, "s2s-latency", job)

    return counter

def create_vivaldi_coord(viv_dict):
    vec = viv_dict.get("vector")
    hgt = viv_dict.get("height")
    err = viv_dict.get("error")
    viv = VivaldiCoordinate(len(vec))
    viv.vector = np.array(vec)
    viv.height = float(hgt)
    viv.error = float(err)
    return viv


def check_service_to_user_constraints(job, node_id, container_id, port, coord_ip_mapping, geo_violations_ctr, latency_violations_ctr):
    constraints = job.get('constraints')
    for constraint in constraints:
        constraint_type = constraint.get(TYPE)
        if constraint_type == 'geo':
            print("###################### S2U GEO ######################")
            geo_violations_ctr = check_s2u_geo_constraint(node_id, geo_violations_ctr, job, constraint)
        elif constraint_type == 'latency':
            print("###################### S2U LATENCY ######################")
            latency_violations_ctr, coord_ip_mapping = check_s2u_latency_constraint(node_id, latency_violations_ctr,
                                                                                    container_id, port, job, constraint,
                                                                                    coord_ip_mapping)
        else:
            raise AttributeError(f"no such constraint type {constraint_type}")

    return geo_violations_ctr, latency_violations_ctr


def check_s2u_geo_constraint(node_id, counter, job, constraint):
    constraint_lat, constraint_long = constraint["location"].split(",")
    threshold = constraint["threshold"]
    print(f"Check geo constraint location {constraint_lat},{constraint_long} with threshold {threshold}km")
    # Allowed violations
    node_lat, node_long = coordinates.get_coordinates()
    distance = geopy.distance.distance((constraint_lat, constraint_long), (float(node_lat), float(node_long))).km
    print(f"Distance between node ({node_lat},{node_long}) and constraint location: {distance}km")
    if distance > threshold:
        print("Distance larger than threshold. Increment violation counter.")
        counter[f"{constraint_lat},{constraint_long}"] += 1
    if counter[f"{constraint_lat},{constraint_long}"] >= ALLOWED_VIOLATIONS:
        print(f"Exceeded violation threshold of {ALLOWED_VIOLATIONS}. Trigger SLA alarm.")
        # Send alarm to cluster orchestrator
        publish_sla_alarm(node_id, "s2u-geo", job)

    return counter


def check_s2u_latency_constraint(node_id, counter, container_id, port, job, constraint, coord_ip_mapping):
    # coord_ip_mapping = {}
    area = constraint['area'].lower()
    print(f"Check latency constraint for area {area}")
    # Sniff on node ip and service port and query GeoLite2 location for src IP addresses
    coord_ip_mapping = {**coord_ip_mapping, **listen_to_port(container_id, port)}
    # Check whether any connected IP is located within the area specified in the SLA
    for coord, ips in coord_ip_mapping.items():
        lat, long = coord.split(",")
        request_location = Point(float(lat), float(long))
        if area in AREAS:
            # Check if request_location is within constraint area
            if AREAS[area].contains(request_location) or request_location.touches(AREAS[area]):
                print(f"Requests from {ips} come from area specified in SLA: {area}. Measure latencies...")
                # In this case we have to measure the latency to the ips in this area
                counter = measure_s2u_latency_violations(node_id, job, ips, constraint, counter)
            else:
                print(f"Requests from {ips} are not located within area specified in SLA. Ignore them...")
        else:
            raise AttributeError(f"Area '{area}' is not supported.")

    # Remove violating IPs from monitoring
    for ip, violations in counter.items():
        if violations > ALLOWED_VIOLATIONS:
            coord_ip_mapping.pop(ip, None)

    return counter, coord_ip_mapping


def listen_to_port(container, port):
    global geolocation_cache
    print(f"Cache: {geolocation_cache}")
    interface = 'any'
    # worker_public_ip = os.environ.get("WORKER_PUBLIC_IP")
    worker_public_ip = PUBLIC_IP
    display_filter = f'http.host == {worker_public_ip}:{port} and http'
    capture = LiveCapture(interface=interface, display_filter=display_filter)
    print(f"Listening on {interface} filtering by {display_filter}")
    capture.sniff(timeout=8)
    capture_size = len(capture)
    location_ip_mapping = {}
    captured_info = []
    if capture_size >= 1:
        for i in range(capture_size):
            packet = capture[i]
            try:
                src_ip = packet.ip.src
                dst_ip = packet.ip.dst
                host = packet['http'].host
                #dst_addr, dst_port = str(host).split(":")
                request_uri = packet['http'].request_uri
                captured_info.append({"src": src_ip, "dst": dst_ip, "host": host, "uri": request_uri})
                # localtime = time.asctime(time.localtime(time.time()))
                # print(f"{container}: {localtime} {src_ip} -> {dst_ip} URI: {request_uri} Host: {host}")

            except AttributeError as e:
                # ignore other packets
                pass
        # Get geolocations of captured IPs
        print(f"Captured: {captured_info}")
        public_src_ips = [e["src"] for e in captured_info if not ipaddress.ip_address(e["src"]).is_private]
        # Remove duplicates
        public_src_ips = list(dict.fromkeys(public_src_ips))
        print(f"Get geolocation for {public_src_ips}")
        ip_locations = {}
        for ip in public_src_ips:
            if ip in geolocation_cache:
                print(f"Cache hit for {ip}: {geolocation_cache[ip]}")
                ip_locations[ip] = geolocation_cache[ip]
        # remove cached ips from request list
        public_src_ips = [e for e in public_src_ips if e not in ip_locations]
        if len(public_src_ips) >= 1:
            # Geolocation
            cluster_ip = os.environ.get("CLUSTER_MANAGER_IP")
            request_address = f"http://{cluster_ip}:10000/api/geolocation"
            response = requests.post(request_address, json=json.dumps(public_src_ips))
            ip_locations = {**ip_locations, **json.loads(response.text)}
            geolocation_cache = {**geolocation_cache, **ip_locations}

        print(ip_locations)
        for ip, loc in ip_locations.items():
            location_key = f"{loc['lat']},{loc['long']}"
            if (location_key in location_ip_mapping and src_ip not in location_ip_mapping[
                location_key]) or location_key not in location_ip_mapping:
                location_ip_mapping.setdefault(location_key, []).append(ip)
        print(f"Loc IP Mapping {location_ip_mapping}")

    print(f"done sniffing on {container}")
    capture.close()
    return location_ip_mapping


def measure_s2u_latency_violations(node_id, job, ips, constraint, violation_ctr):
    threshold = constraint['threshold']
    # {ip1: <min rtt>, ip2: <min rtt>, ...}
    statistics = network_measurement.parallel_ping_retry(ips)

    print(f"Ping stats: {statistics}")
    # Measurement tolerance
    tol = 0.2
    # Allowed violations
    violations = {}

    # check if any ping exceeds threshold
    for ip, rtt in statistics.items():
        if rtt > threshold + (threshold * tol):
            violation_ctr[ip] += 1
            if violation_ctr[ip] >= ALLOWED_VIOLATIONS:
                violations[ip] = rtt

    print(f"Violations: {violations} Violation Counter: {violation_ctr}")
    if len(violations) >= 1:
        print(f"Exceeded violation threshold of {ALLOWED_VIOLATIONS}. Trigger SLA alarm.")
        # Send alarm to cluster orchestrator
        # violated_job = job.copy()
        # violated_job['constraints'] = constraint
        # ip_rtt_stats = {"fulfilled": statistics, "violated": violations}
        publish_sla_alarm(node_id, "s2u-latency", job, violations)
        # {"requests": {"ip1": xxms, "ip2": yyms}, "violations": {"ip3: zzms}"
    return violation_ctr



def remove_violations_from_counter(counter):
    # Remove violating IPs from monitoring
    # Note that list(data.items()) creates a shallow copy of the items of the dictionary, i.e.a new list containing
    # references to all keys and values, so it's safe to modify the original dict inside the loop.
    for k, v in list(counter.items()):
        if v > ALLOWED_VIOLATIONS:
            del counter[k]
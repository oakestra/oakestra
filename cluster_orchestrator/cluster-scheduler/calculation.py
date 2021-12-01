import json
import random
import time

import numpy as np
import requests

from mongodb_client import mongo_find_one_node, mongo_find_all_active_nodes, mongo_find_node_by_name, mongo_find_all_nodes, mongo_find_node_by_id
from shapely.geometry import Point, Polygon
from shapely.ops import nearest_points
import geopy.distance
from NetworkCoordinateSystem.network_measurements import parallel_ping
from NetworkCoordinateSystem.vivaldi_coordinate import VivaldiCoordinate
from NetworkCoordinateSystem.multilateration import multilateration

# TODO: Temporary area mapping until we know how we select area in SLA
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


def calculate(job, is_sla_violation=False, source_client_id=None, worker_ip_rtt_stats=None):
    print('calculating...')
    # check here if job has any user preferences, e.g. on a specific node, a specific cpu architecture,
    constraints = job.get('constraints')
    if job.get('node'):
        # TODO: add memory cpu reqs to constraint?
        return deploy_on_desired_node(job=job)
    elif len(constraints) >= 1:
        # First filter out nodes that can't provide required resources
        qualified_nodes = filter_nodes_based_on_resources(job=job)
        # Then find nodes that can fulfill the SLA constraints
        return constraint_based_scheduling(qualified_nodes, job, is_sla_violation, source_client_id, worker_ip_rtt_stats)
    else:
        return greedy_load_balanced_algorithm(job=job)


def first_fit_algorithm(job):
    """Which of the worker nodes fits the Qos of the deployment file as the first"""
    active_nodes = mongo_find_all_active_nodes()

    print('active_nodes: ')
    for node in active_nodes:
        print(node)

        available_cpu = node.get('current_cpu_cores_free')
        available_memory = node.get('current_free_memory_in_MB')
        node_info = node.get('node_info')
        # technology = node_info.get('technology')
        virtualization = node_info.get('virtualization')

        job_req = job.get('requirements')
        if available_cpu >= job_req.get('cpu') and available_memory >= job_req.get('memory') and job.get(
                'virtualization') in virtualization:
            return 'positive', node

    # no node found
    return 'negative', 'FAILED_NoCapacity'


def deploy_on_desired_node(job):
    # job_req = job.get('requirements')
    job_req = {'memory': job.get('memory'), 'vcpus': job.get('vcpus')}
    desired_node = job_req.get('node')
    node = mongo_find_node_by_name(desired_node)

    if node is None or node == "Error":
        return 'negative', 'FAILED_Node_Not_Found'

    available_cpu = node.get('current_cpu_cores_free')
    available_memory = node.get('current_free_memory_in_MB')

    if available_cpu >= job_req.get('cpu') and available_memory >= job_req.get('memory'):
        return "positive", node

    return 'negative', 'FAILED_Desired_Node_No_Capacity'


def greedy_load_balanced_algorithm(job):
    """Which of the nodes within the cluster have the most capacity for a given job"""

    # job_req = job.get('requirements')
    job_req = {'memory': job.get('memory'), 'vcpus': job.get('vcpus')}

    active_nodes = mongo_find_all_active_nodes()
    qualified_nodes = []

    for node in active_nodes:
        print(node)
        available_cpu = float(node.get('current_cpu_cores_free'))
        available_memory = float(node.get('current_free_memory_in_MB'))
        node_info = node.get('node_info')
        virtualization = node_info.get('virtualization')

        if available_cpu >= float(job_req.get('vcpus')) \
                and available_memory >= int(job_req.get('memory')) \
                and job.get('virtualization') in virtualization:
            qualified_nodes.append(node)

    target_node = None
    target_cpu = 0
    target_mem = 0

    # return if no qualified clusters found
    if len(qualified_nodes) < 1:
        return 'negative', 'NoActiveClusterWithCapacity'

    # return the cluster with the most cpu+ram
    for node in qualified_nodes:
        cpu = float(node.get('current_cpu_cores_free'))
        mem = float(node.get('current_free_memory_in_MB'))
        if cpu >= target_cpu and mem >= target_mem:
            target_cpu = cpu
            target_mem = target_cpu
            target_node = node

    return 'positive', target_node


def replicate(job):
    return 1

def filter_nodes_based_on_resources(job):
    active_nodes = mongo_find_all_active_nodes()
    qualified_nodes = []

    for node in active_nodes:
        print(node)
        available_cpu = float(node.get('current_cpu_cores_free'))
        available_memory = float(node.get('current_free_memory_in_MB'))
        node_info = node.get('node_info')
        virtualization = node_info.get('virtualization')

        if available_cpu >= float(job.get('vcpus')) \
                and available_memory >= int(job.get('memory')) \
                and job.get('virtualization') in virtualization:
            qualified_nodes.append(node)

    # return if no qualified clusters found
    if len(qualified_nodes) < 1:
        return 'negative', 'NoActiveClusterWithCapacity'

    return qualified_nodes


def constraint_based_scheduling(qualified_nodes, job, is_sla_violation, source_client_id, worker_ip_rtt_stats):
    constraints = job.get('constraints')
    target_nodes = []
    node_constraint_mapping = {}
    for constraint in constraints:
        constraint_type = constraint.get('type')
        if constraint_type == 'latency':
            # If this is the initial service deployment there are no user latency information available yet.
            # Do an initial placement based on geographical proximity
            if is_sla_violation:
                nodes = latency_constraint_scheduling(qualified_nodes, source_client_id, worker_ip_rtt_stats)
                print(f"latency scheduling result: {nodes}")
            else:
                nodes = initial_latency_constraint_scheduling(qualified_nodes, constraint)#
                print(f"Initial latency scheduling result: {nodes}")
        elif constraint_type == 'geo':
            nodes = geo_based_scheduling(qualified_nodes, constraint)
        else:
            print(f"Unknown constraint type: {constraint_type}")
            return 'negative', 'NoSuchConstraintType'
        for node in nodes:
            node_constraint_mapping.setdefault(node.get('_id'), []).append(constraint)
            # if node not in target_nodes:
            #     target_nodes.append(node)

    print(f"NODE_CONSTRAINT_MAPPING: {node_constraint_mapping}")
    # Ignore nodes that cannot fulfill all constraints
    for key, value in node_constraint_mapping.items():
        if len(value) == len(constraints):
            node = [n for n in qualified_nodes if n.get('_id') == key]
            target_nodes.append(*node)

    if len(target_nodes) == 0:
        return 'negative', 'NoNodeFulfillsSLAConstraints'

    return 'positive', target_nodes

def initial_latency_constraint_scheduling(qualified_nodes, constraint):
    """
    The initial placement is done based on geographical proximity to have a good chance for a node with low latency.
    Once the service was deployed to one or more nodes, the respective nodes have to monitor whether they fulfill the
    SLA and trigger a service replication/migration if that is not the case.
    The target node for the replication/migration is then based on latency. The node currently running the service,
    the cluster orchestrator, and another random node ping the user and determine the closest node to the calculated
    user position based on Vivaldi proximity.
    """
    area = constraint.get('area')
    if area.lower() in AREAS:
        area_geo = AREAS[area.lower()]
    else:
        raise ValueError("Invalid area")

    node_area_distances = []
    for node in qualified_nodes:
        node_location = Point(float(node.get('lat')), float(node.get('long')))
        # Check whether node is located within speciefied area
        if area_geo.contains(node_location) or area_geo.touches(node_location):
            node_area_distances.append((-1, node))
        else:
            # Otherwise calculate shortest distance from the node to the area
            p1, _ = nearest_points(node_location, area_geo)
            node_area_distance = geopy.distance.distance((p1.x, p1.y), (node_location.x, node_location.y)).km
            node_area_distances.append((node_area_distance, node))

    # Sort lists by distance
    node_area_distances.sort(key=lambda x: x[0])

    # return nodes sorted by distance
    return [dist_node_tuple[1] for dist_node_tuple in node_area_distances]

def geo_based_scheduling(qualified_nodes, constraint):
    location = constraint.get('location')  # lat,long
    lat, long = location.split(",")
    shapely_location = Point(float(lat), float(long))
    threshold = constraint.get('threshold')

    # TODO: adapt naming for qualified_nodes in previous filtering -> nodes with resources
    nodes = []
    for node in qualified_nodes:
        node_location = Point(float(node.get('lat')), float(node.get('long')))
        node_location_distance = geopy.distance.distance((shapely_location.x, shapely_location.y), (node_location.x, node_location.y)).km

        # check if distance is smaller or equal to specified threshold
        if node_location_distance <= threshold:
            nodes.append(node)

    # Return all nodes that are within the specified threshold
    return nodes

def latency_constraint_scheduling(qualified_nodes, source_client_id, worker_ip_rtt_stats):
    multilateration_data = []
    # nodes = list(mongo_find_all_nodes())
    nodes = qualified_nodes
    nodes_size = len(nodes)
    # Case 1: Cluster has only one worker node therefore we cannot replicate the service
    if nodes_size == 1:
        print(f"Cannot migrate service because cluster only has one worker.")
    elif nodes_size >= 2:
        target_node = random.choice([node for node in nodes if str(node.get('_id')) != source_client_id])
        target_client_id = str(target_node.get('_id'))
        # Case 2: Cluster has two nodes. Just ask scheduler if other node can run the service
        if nodes_size == 2:
            print(f"Cluster has only one other suitable worker. Try deploying service to that node.")
            return [target_node]
        # Case 3: Cluster has more than two nodes. In that case we have to tell a random node to ping the target IP
        else:
            print(f"Cluster has {nodes_size - 1} other suitable nodes. Find best target via NCS. ")
            # Add vivaldi and ping info of cluster orchestrator to data for multilateration
            co_ip_rtt_stats = parallel_ping(worker_ip_rtt_stats.keys())
            # Cluster orchestartor is passive member of network coordinate systems and therefore always remains in the
            # origin because it does not actively ping other nodes and updates its position.
            multilateration_data.append((np.array([0.0, 0.0]), co_ip_rtt_stats))
            address = None
            worker1_vivaldi_coord = None
            worker2_vivaldi_coord = None
            # Build Vivaldi network to find closest node later
            vivaldi_network = {}
            for n in nodes:
                # Add node to Vivaldi network
                viv_vector = n.get('vivaldi_vector')
                viv_height = n.get('vivaldi_height')
                viv_error = n.get('vivaldi_error')
                vivaldi_coord = VivaldiCoordinate(len(viv_vector))
                vivaldi_coord.vector = viv_vector
                vivaldi_coord.height = viv_height
                vivaldi_coord.error = viv_error
                vivaldi_network[n.get("_id")] = vivaldi_coord

                if str(n.get('_id')) == source_client_id:
                    # Add vivaldi and ping info of first worker to data for multilateration
                    worker1_ip_rtt_stats = worker_ip_rtt_stats
                    worker1_vivaldi_coord = vivaldi_coord
                    print(f"Worker 1 Vivaldi: {worker1_vivaldi_coord}")
                    multilateration_data.append((worker1_vivaldi_coord.vector, worker1_ip_rtt_stats))
                elif str(n.get('_id')) == target_client_id:
                    address = f"http://{n.get('node_address')}:{n.get('node_info').get('port')}/monitoring/ping"
                    worker2_vivaldi_coord = vivaldi_coord
                    print(f"Worker 2 Vivaldi: {worker2_vivaldi_coord}")
                    # Add vivaldi and ping info of second worker to data for multilateration
                    print(f"Get ping result for {list(worker_ip_rtt_stats.keys())} from {address}")
                    try:
                        if address is None:
                            raise ValueError("No address found")
                        response = requests.post(address, json=json.dumps(list(worker_ip_rtt_stats.keys())))
                        worker2_ip_rtt_stats = response.json()
                        multilateration_data.append((worker2_vivaldi_coord.vector, worker2_ip_rtt_stats))
                    except requests.exceptions.RequestException as e:
                        print('Calling node /ping not successful')

            if worker1_vivaldi_coord is None or worker2_vivaldi_coord is None:
                raise ValueError("No Vivaldi information available.")

            # Approximate locations of IPs in Vivaldi network
            print(f"Start multilateration: {multilateration_data}")
            ip_locations = multilateration(multilateration_data)
            print(f"Multilateration result: {ip_locations}")

            # In case of multiple violating request locations, we want to the node closest to the point which minimizes
            # the sum of distances to the violating request locations. This point is known as the Geometric Median and
            # does not have a closed form solution but only approximation approaches. This implementation uses the
            # Weiszfeld's algorithm which is a form of iteratively re-weighted least squares.
            if len(ip_locations) == 1:
                closest_worker_id = find_closest_vivaldi_coord(list(ip_locations.values())[0], vivaldi_network)
            else:
                min_dist_point = shortest_dists(list(ip_locations.values()))
                closest_worker_id = find_closest_vivaldi_coord(min_dist_point, vivaldi_network)

            print(f"Closest worker: {closest_worker_id}")
            closest_worker = mongo_find_node_by_id(closest_worker_id)
            return [closest_worker]

    else:
        print(f"No active nodes.")


def shortest_dists(pts):
    old = np.random.rand(2)
    new = np.array([0, 0])
    while np.linalg.norm(old - new) > 1e-6:
        num = 0
        denom = 0
        for i in range(len(pts)):
            dist = np.linalg.norm(new - pts[i, :])
            num += pts[i, :] / dist
            denom += 1 / dist
        old = new
        new = num / denom

    print(f"Minimum distance point: {new}")
    return new


def find_closest_node(node, nodes):
    nodes = np.asarray(nodes)
    deltas = nodes - node
    dist_2 = np.einsum('ij,ij->i', deltas, deltas)
    return nodes[np.argmin(dist_2)]

def find_closest_vivaldi_coord(point, candidates):
    deltas = [v.vector for v in candidates.values()] - np.array(point)
    # Calc euclidean distance
    dist_2 = np.sum(deltas**2, axis=1)
    dist = np.sqrt(dist_2)
    # Add vivaldi heights
    dist += np.array([v.height for v in candidates.values()])
    return list(candidates.keys())[np.argmin(dist)]



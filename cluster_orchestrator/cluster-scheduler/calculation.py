import json
import random
import time

import numpy as np
import requests

from mongodb_client import *
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
    connectivity = job.get('connectivity')
    target_node = job.get('target_node')
    if target_node:
        print(f"Deploy to specified node: {target_node}")
        return check_suitability_of_target(job=job)
    else:
        if len(constraints) >= 1 or len(connectivity) >= 1:
            print(f"Find node based on specified constraints")
            # First filter out nodes that can't provide required resources
            qualified_nodes = filter_nodes_based_on_resources(job=job)
            # Then find nodes that can fulfill the SLA constraints
            return constraint_aware_scheduling(qualified_nodes, job, is_sla_violation, source_client_id, worker_ip_rtt_stats)
        else:
            return greedy_load_balanced_algorithm(job=job)


def first_fit_algorithm(job):
    """Which of the worker nodes fits the Qos of the deployment file as the first"""
    active_nodes = mongo_find_all_active_nodes()

    print('active_nodes: ')
    for node in active_nodes:

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


def check_suitability_of_target(job):
    # job_req = job.get('requirements')
    job_req = {'memory': job.get('memory'), 'vcpus': job.get('vcpus')}
    desired_node = job.get('target_node')
    node = mongo_find_node_by_name(desired_node)
    print(f"Node: {node.get('_id')}")
    if node is None or node == "Error":
        return 'negative', 'FAILED_Node_Not_Found', job

    available_cpu = node.get('current_cpu_cores_free')
    available_memory = node.get('current_free_memory_in_MB')

    if available_cpu >= job_req.get('vcpus') and available_memory >= job_req.get('memory'):
        return "positive", node, job

    return 'negative', 'FAILED_Desired_Node_No_Capacity'


def greedy_load_balanced_algorithm(job):
    """Which of the nodes within the cluster have the most capacity for a given job"""

    # job_req = job.get('requirements')
    job_req = {'memory': job.get('memory'), 'vcpus': job.get('vcpus')}

    active_nodes = mongo_find_all_active_nodes()
    qualified_nodes = []

    for node in active_nodes:
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
        return 'negative', 'NoActiveClusterWithCapacity', job

    # return the cluster with the most cpu+ram
    for node in qualified_nodes:
        cpu = float(node.get('current_cpu_cores_free'))
        mem = float(node.get('current_free_memory_in_MB'))
        if cpu >= target_cpu and mem >= target_mem:
            target_cpu = cpu
            target_mem = mem
            target_node = node

    return 'positive', target_node, job


def replicate(job):
    return 1

def filter_nodes_based_on_resources(job):
    active_nodes = list(mongo_find_all_active_nodes())
    qualified_nodes = []
    print(f"Active Nodes: {[str(n.get('_id')) for n in active_nodes]}")
    for node in active_nodes:
        available_cpu = float(node.get('current_cpu_cores_free'))
        available_memory = float(node.get('current_free_memory_in_MB'))
        node_info = node.get('node_info')
        virtualization = node_info.get('virtualization')

        if available_cpu >= float(job.get('vcpus')) \
                and available_memory >= int(job.get('memory')) \
                and job.get('virtualization') in virtualization:
            qualified_nodes.append(node)

    return qualified_nodes


def create_worker_id_vivaldi_info_mapping():
    node_id_viv_mapping = {}
    # Get Vivaldi info and transform to network
    vivaldi_info = mongo_get_vivaldi_data()
    for viv in vivaldi_info:
        node_id = str(viv.get("_id"))
        vec = viv.get("vivaldi_vector")
        err = viv.get("vivaldi_error")
        hgt = viv.get("vivaldi_height")
        dim = len(vec)
        vivaldi_coord = VivaldiCoordinate(dim)
        vivaldi_coord.vector = vec
        vivaldi_coord.error = err
        vivaldi_coord.height = hgt
        node_id_viv_mapping[node_id] = vivaldi_coord

    return node_id_viv_mapping

def create_worker_id_geolocation_info_mapping():
    node_id_geolocation_mapping = {}
    geoloc_data = mongo_get_geolocation_data()
    for d in geoloc_data:
        node_id = str(d.get("_id"))
        lat = d.get("lat")
        long = d.get("long")
        if lat is not None and long is not None:
            node_id_geolocation_mapping[node_id] = np.array([float(lat), float(long)])

    return node_id_geolocation_mapping

def constraint_aware_scheduling(qualified_nodes, job, is_sla_violation, source_client_id, worker_ip_rtt_stats):
    # In case of a SLA violation the service should not be deployed to the violating worker.
    print(f"Qualified nodes: {[str(n.get('_id')) for n in qualified_nodes]}")
    qualified_nodes = service_to_user_constraint_scheduling(qualified_nodes, job, is_sla_violation, source_client_id, worker_ip_rtt_stats)
    print(f"Qualified nodes after S2U-filtering: {[str(n.get('_id')) for n in qualified_nodes]}")
    qualified_nodes, augmented_job = service_to_service_constraint_scheduling(qualified_nodes, job, source_client_id)
    print(f"Qualified nodes after S2S-filtering: {[str(n.get('_id')) for n in qualified_nodes]}")

    if len(qualified_nodes) == 0:
        return 'negative', 'NoNodeFulfillsSLAConstraints', job

    # return first suitable node
    # shuffled_nodes = qualified_nodes.copy()
    # random.shuffle(shuffled_nodes)
    # return 'positive', shuffled_nodes[0], augmented_job
    return 'positive', qualified_nodes[0], augmented_job

def service_to_service_constraint_scheduling(qualified_nodes, job, source_client_id=None):
    # The worker on which the service might be deployed requires the public IP addresses and the location coordinates
    # of the workers running the target microservices, such that they can monitor the SLA fulfillment.
    augmented_job = job.copy()
    connectivity = job.get('connectivity')
    if len(connectivity) == 0 or len(qualified_nodes) == 0:
        return qualified_nodes, job
    node_id_viv_mapping = create_worker_id_vivaldi_info_mapping()
    node_id_geoloc_mapping = create_worker_id_geolocation_info_mapping()
    node_constraint_mapping = {}
    nr_con_constraints = 0
    for i, con in enumerate(connectivity):
        con_constraints = con.get("con_constraints")
        nr_con_constraints += len(con_constraints)
        # Find worker that runs the target microservice
        target_application_id = job.get("applicationID")
        target_microservice_id = con.get("target_microservice_id")
        target_microservice_job = mongo_find_job_by_microservice_id(target_application_id, target_microservice_id)
        instance_list = target_microservice_job.get("instance_list")
        # Handle service-to-service constraints
        for constraint in con_constraints:
            constraint_type = constraint.get("type")
            if len(instance_list) != 0:
                target_worker_id = instance_list[0]["worker_id"]
                target_worker = mongo_find_node_by_id(target_worker_id)
                target_worker_info = {"id": target_worker_id,
                                      "applicationID": target_application_id,
                                      "microserviceID": target_microservice_id,
                                      "ip": target_worker.get("public_ip"),
                                      "loc": node_id_geoloc_mapping[target_worker_id].tolist()}
                augmented_job["connectivity"][i]["target_worker_info"] = target_worker_info
                worker_ids_in_range = []
                # In case of a latency constraint check which nodes are within threshold to worker that runs the
                # referenced service based on Vivaldi network distance
                threshold = constraint.get("threshold")
                tol = 0.2
                qualified_node_ids = [str(n.get("_id")) for n in qualified_nodes]
                if constraint_type == "latency":
                    target_worker_vivaldi_coord = node_id_viv_mapping[target_worker_id]
                    qualified_vivaldi_coords = {node_id: viv_coord for node_id, viv_coord in node_id_viv_mapping.items()
                                                if node_id in qualified_node_ids}
                    worker_ids_in_range = find_vivaldi_coords_in_range(target_worker_vivaldi_coord.vector, threshold,
                                                                       tol, qualified_vivaldi_coords)
                elif constraint_type == "geo":
                    target_worker_geo_coords = node_id_geoloc_mapping[target_worker_id]
                    qualified_geo_coords = {node_id: geo_coord for node_id, geo_coord in node_id_geoloc_mapping.items()
                                            if node_id in qualified_node_ids}
                    worker_ids_in_range = find_geolocations_in_range(target_worker_geo_coords, threshold,
                                                                     qualified_geo_coords)

                # Add fulfilled constraints to
                for worker_id in worker_ids_in_range:
                    # Don't consider violating worker in case of a SLA violation
                    if source_client_id is not None and worker_id == source_client_id:
                        continue
                    node_constraint_mapping.setdefault(worker_id, []).append(constraint)
            else:
                return []
    # print(f"S2S node constraint mapping:\n")
    # print_constraint_mapping(node_constraint_mapping, is_s2s=True, target_id=target_worker_id)
    # Filter nodes that don't satisfy all connectivity constraints
    s2s_qualified_node_ids = []
    for node_id, constraints in node_constraint_mapping.items():
        if nr_con_constraints == len(constraints):
            s2s_qualified_node_ids.append(node_id)

    return [n for n in qualified_nodes if str(n.get("_id")) in s2s_qualified_node_ids], augmented_job


def service_to_user_constraint_scheduling(qualified_nodes, job, is_sla_violation, source_client_id, worker_ip_rtt_stats):
    constraints = job.get('constraints')
    if len(constraints) == 0 or len(qualified_nodes) == 0:
        return qualified_nodes
    nr_constraints = len(constraints)
    node_constraint_mapping = {}
    # Handle service-to-user constraints
    for constraint in constraints:
        constraint_type = constraint.get('type')
        if constraint_type == 'latency':
            # If this is the initial service deployment there are no user latency information available yet.
            # Do an initial placement based on geographical proximity
            if is_sla_violation:
                nodes = sla_alarm_latency_constraint_scheduling(constraint, qualified_nodes, source_client_id, worker_ip_rtt_stats)
                print(f"latency scheduling result: {[str(n.get('_id')) for n in nodes]}")
            else:
                nodes = initial_latency_constraint_scheduling(constraint, qualified_nodes)
                print(f"Initial latency scheduling result: {[str(n.get('_id')) for n in nodes]}")
        elif constraint_type == 'geo':
            nodes = geo_based_scheduling(qualified_nodes, constraint)
        else:
            print(f"Unknown constraint type: {constraint_type}")
            return []
        for node in nodes:
            node_constraint_mapping.setdefault(str(node.get('_id')), []).append(constraint)
    # return node_constraint_mapping
    print(f"S2U node constraint mapping:\n")
    # print_constraint_mapping(node_constraint_mapping)
    # Filter nodes that don't satisfy all connectivity constraints
    s2u_qualified_node_ids = []
    for node_id, cons in node_constraint_mapping.items():
        if nr_constraints == len(cons):
            s2u_qualified_node_ids.append(node_id)

    return [n for n in qualified_nodes if str(n.get("_id")) in s2u_qualified_node_ids]


def initial_latency_constraint_scheduling(constraint, qualified_nodes):
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
    workers_inside_area = []
    for node in qualified_nodes:
        node_location = Point(float(node.get('lat')), float(node.get('long')))
        # Check whether node is located within specified area
        if area_geo.contains(node_location) or area_geo.touches(node_location):
            workers_inside_area.append(node)
        else:
            # Otherwise calculate shortest distance from the node to the area
            p1, _ = nearest_points(node_location, area_geo)
            node_area_distance = geopy.distance.distance((p1.x, p1.y), (node_location.x, node_location.y)).km
            node_area_distances.append((node_area_distance, node))

    # Sort lists by distance
    node_area_distances.sort(key=lambda x: x[0])

    # if workers exists that are located within th specified area, return these workers
    if len(workers_inside_area) > 0:
        return workers_inside_area
    # otherwise, return the closest worker
    else:
        return [dist_node_tuple[1] for dist_node_tuple in node_area_distances][0]

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

def sla_alarm_latency_constraint_scheduling(constraint, qualified_nodes, source_client_id, worker_ip_rtt_stats):
    multilateration_data = {}
    # nodes = list(mongo_find_all_nodes())
    all_nodes = list(mongo_find_all_nodes())
    qualified_nodes_size = len(qualified_nodes)
    # Case 1: Cluster has only one worker node therefore we cannot replicate the service
    if qualified_nodes_size == 1:
        print(f"Cannot migrate service because cluster only has one qualified worker.")
    elif qualified_nodes_size >= 2:
        vivaldi_dim = len(all_nodes[0].get("vivaldi_vector"))
        # In n-dimensional network (n+1) measurements are required to perform multilateration. CO and source worker node
        # are always reference nodes. Hence, for n-dimensional network we need (n+1)-2=n-1 other random reference nodes.
        nr_rnd_other_nodes = vivaldi_dim - 1
        rnd_other_nodes = random.sample([node for node in qualified_nodes if str(node.get('_id')) != source_client_id], nr_rnd_other_nodes)
        target_client_ids = [str(n.get('_id')) for n in rnd_other_nodes]
        #rnd_other_node = random.choice([node for node in qualified_nodes if str(node.get('_id')) != source_client_id])
        #target_client_id = str(rnd_other_node.get('_id'))
        # Case 2: Cluster has two nodes. Just ask scheduler if other node can run the service
        if qualified_nodes_size == 2:
            print(f"Cluster has only one other suitable worker. Try deploying service to that node.")
            return [rnd_other_nodes[0]]
        # Case 3: Cluster has more than two nodes. In that case we have to tell a random node to ping the target IP
        else:
            print(f"Cluster has {qualified_nodes_size - 1} other suitable nodes. Find best target via NCS. ")
            # Add vivaldi and ping info of cluster orchestrator to data for multilateration
            co_ip_rtt_stats = parallel_ping(worker_ip_rtt_stats.keys())
            # Cluster orchestartor is passive member of network coordinate systems and therefore always remains in the
            # origin because it does not actively ping other nodes and updates its position.
            multilateration_data["CO"] = (np.zeros(vivaldi_dim), co_ip_rtt_stats)
            address = None
            worker1_vivaldi_coord = None
            rnd_worker_vivaldi_coord = None
            # Build Vivaldi network to find closest node later
            vivaldi_network = {}
            for n in all_nodes:
                viv_vector = n.get('vivaldi_vector')
                viv_height = n.get('vivaldi_height')
                viv_error = n.get('vivaldi_error')
                vivaldi_coord = VivaldiCoordinate(len(viv_vector))
                vivaldi_coord.vector = viv_vector
                vivaldi_coord.height = viv_height
                vivaldi_coord.error = viv_error
                if str(n.get('_id')) == source_client_id:
                    # Add vivaldi and ping info of first worker to data for multilateration
                    worker1_ip_rtt_stats = worker_ip_rtt_stats
                    worker1_vivaldi_coord = vivaldi_coord
                    print(f"Worker 1 Vivaldi: {worker1_vivaldi_coord}")
                    multilateration_data[str(n.get("_id"))] = (worker1_vivaldi_coord.vector, worker1_ip_rtt_stats)
                else:
                    # Add node to Vivaldi network
                    vivaldi_network[str(n.get("_id"))] = vivaldi_coord
                    if str(n.get('_id')) in target_client_ids:
                        address = f"http://{n.get('node_address')}:{n.get('node_info').get('port')}/monitoring/ping"
                        rnd_worker_vivaldi_coord = vivaldi_coord
                        print(f"Random Worker Vivaldi: {rnd_worker_vivaldi_coord}")
                        # Add vivaldi and ping info of second worker to data for multilateration
                        print(f"Get ping result for {list(worker_ip_rtt_stats.keys())} from {address}")
                        try:
                            if address is None:
                                raise ValueError("No address found")
                            response = requests.post(address, json=json.dumps(list(worker_ip_rtt_stats.keys())))
                            rnd_worker_ip_rtt_stats = response.json()
                            multilateration_data[str(n.get("_id"))] = (rnd_worker_vivaldi_coord.vector, rnd_worker_ip_rtt_stats)
                        except requests.exceptions.RequestException as e:
                            print('Calling node /ping not successful')

            print(multilateration_data)
            for viv, _ in multilateration_data.values():
                if viv is None:
                    print(viv)
                    raise ValueError("No Vivaldi information available.")

            # Approximate locations of IPs in Vivaldi network
            print(f"Start multilateration: {multilateration_data}")
            ip_locations = multilateration(list(multilateration_data.values()))
            print(f"Multilateration result: {ip_locations}")

            # In case of multiple violating request locations, we want to the node closest to the point which minimizes
            # the sum of distances to the violating request locations. This point is known as the Geometric Median and
            # does not have a closed form solution but only approximation approaches. This implementation uses the
            # Weiszfeld's algorithm which is a form of iteratively re-weighted least squares.
            threshold = constraint.get("threshold")
            range = threshold
            tol = 0.2
            if len(ip_locations) == 1:
                reference_point = list(ip_locations.values())[0]
                qualified_worker_ids = find_vivaldi_coords_in_range(reference_point, range, tol, vivaldi_network)
                # closest_worker_id = find_closest_vivaldi_coord(list(ip_locations.values())[0], vivaldi_network)
            else:
                min_dist_point = shortest_dists(list(ip_locations.values()))
                qualified_worker_ids = find_vivaldi_coords_in_range(min_dist_point, range, tol, vivaldi_network)
                # closest_worker_id = find_closest_vivaldi_coord(min_dist_point, vivaldi_network)

            return [n for n in qualified_nodes if str(n.get("_id")) in qualified_worker_ids and str(n.get("_id")) != source_client_id]

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

def find_geolocations_in_range(point, range, candidates):
    dist = calc_dists_to_point_in_km(point, list(candidates.values()))
    # return IDs of candidates within range
    in_range_indices = np.where(dist <= range)
    return np.array(list(candidates.keys()))[in_range_indices]

def find_vivaldi_coords_in_range(point, range, tol, candidates):
    dist = calc_vivaldi_dists_to_point(point, candidates.values())
    # return IDs of candidates within range
    in_range_indices = np.where(dist <= range + (range * tol))
    return np.array(list(candidates.keys()))[in_range_indices]

def find_closest_vivaldi_coord(point, candidates):
    dist = calc_vivaldi_dists_to_point(point, candidates.values())
    return list(candidates.keys())[np.argmin(dist)]

def calc_vivaldi_dists_to_point(point, candidates):
    deltas = [v.vector for v in candidates] - np.array(point)
    # Calc euclidean distance
    dist_2 = np.sum(deltas**2, axis=1)
    dist = np.sqrt(dist_2)
    # Add vivaldi heights
    dist += np.array([v.height for v in candidates])

    return dist

def calc_dists_to_point_in_km(point, candidates):
    distances = []
    for c in candidates:
        dist = geopy.distance.distance(c, point).km
        distances.append(dist)

    # Calc euclidean distance
    #deltas = candidates - np.array(point)
    #dist_2 = np.sum(deltas**2, axis=1)
    #dist = np.sqrt(dist_2)
    return np.array(distances)


def print_constraint_mapping(constraint_mapping, is_s2s=False, target_id=False):
    table_data = []
    # Collect all constraints
    constraints = []
    first_row = ["Node ID"]
    for cons in constraint_mapping.values():
        for c in cons:
            if c not in constraints:
                constraints.append(c)
                typ = c.get("type")
                if is_s2s:
                    first_row.append(f"{typ} {target_id}")
                else:
                    if typ == "latency":
                        area = c.get("area")
                    elif typ == "geo":
                        area = c.get("location")
                    first_row.append(f"{typ} {area}")
    table_data.append(first_row)
    for node_id, cons in constraint_mapping.items():
        row = [node_id] + ["", ] * len(constraints)
        for c in cons:
            c_idx = constraints.index(c) + 1  # First cell is node id
            row[c_idx] = "X"
        table_data.append(row)

    for row in table_data:
        cells = "{: >20} " * (len(constraints) + 1)
        print(cells.format(*row))
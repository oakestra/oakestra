import time

from mongodb_client import mongo_find_one_node, mongo_find_all_active_nodes, mongo_find_node_by_name
from shapely.geometry import Point, Polygon
from shapely.ops import nearest_points
import geopy.distance

def calculate(job):
    print('calculating...')
    # check here if job has any user preferences, e.g. on a specific node, a specific cpu architecture,
    constraints = job.get('constraints')
    if job.get('node'):
        # TODO: add memory cpu reqs to constraint?
        return deploy_on_desired_node(job=job)
    elif len(constraints) >= 1:
        # First filter out nodes that can't provide required resources
        qualified_nodes = filter_nodes_based_on_resources(job=job)
        # Then choose node that best fulfills the SLA
        # If there isn't a single node that can fulfill the SLA try to deploy to multiple nodes
        return constraint_based_scheduling(qualified_nodes=qualified_nodes, job=job)
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


def constraint_based_scheduling(qualified_nodes, job):
    constraints = job.get('constraints')
    target_nodes = []
    node_constraint_mapping = {}
    for constraint in constraints:
        constraint_type = constraint.get('type')
        if constraint_type == 'latency':
            nodes = latency_based_scheduling(qualified_nodes, constraint)
        elif constraint_type == 'geo':
            nodes = geo_based_scheduling(qualified_nodes, constraint)
        else:
            print("no such constraint type")
            #return 'negative', 'NoSuchConstraintType'
        for node in nodes:
            node_constraint_mapping.setdefault(node.get('_id'), []).append(constraint)
            if node not in target_nodes:
                target_nodes.append(node)


    # Sort by number of constraints fulfilled by the corresponding node
    node_constraint_sorted_keys = sorted(node_constraint_mapping, key=lambda k: len(node_constraint_mapping[k]), reverse=True)
    result_nodes_ids = []
    for key in node_constraint_sorted_keys:
        value = node_constraint_mapping[key]
        # Remove constraints that are fulfilled by the node from the list of constraints
        constraints = [con for con in constraints if con not in value]
        # Add node id to result
        result_nodes_ids.append(key)
        # If remaining list of constraints is empty, we are done
        if len(constraints) == 0:
            break

    # If remaining list of constraints is not empty, there are not enough nodes to cover all constriants
    if len(constraints) >= 1:
        print("cannot fulfill constraints")
        return 'negative', 'NodesCannotFulfillConstraints'

    result_nodes_and_constraints = []
    for node_id in result_nodes_ids:
        for node in target_nodes:
            if node_id == node.get('_id'):
                result_nodes_and_constraints.append((node, node_constraint_mapping[node_id]))

    print(f"target nodes {[(e[0].get('node_info').get('host'), e[1]) for e in result_nodes_and_constraints]}")
    return 'positive', result_nodes_and_constraints

def latency_based_scheduling(qualified_nodes, constraint):
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


# TODO: Temporary area mapping until we know how we select area in SLA
MUNICH = Polygon([[48.24819, 11.50406], [48.22807, 11.63521], [48.18093, 11.69083], [48.1369, 11.72242],
                  [48.07689, 11.68534], [48.06221, 11.50818], [48.13008, 11.38871], [48.15757, 11.36124],
                  [48.20107, 11.39077]])
GARCHING = Polygon([[48.26122, 11.59742], [48.27013, 11.68445], [48.21720, 11.65063], [48.23013, 11.59862]])

AREAS = {
    "munich": MUNICH,
    "garching": GARCHING
}
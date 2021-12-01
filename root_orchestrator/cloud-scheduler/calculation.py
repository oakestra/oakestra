import time

from mongodb_client import mongo_find_cluster_by_location, is_cluster_active, mongo_find_all_active_clusters
import geopy.distance
from shapely.geometry import Point, Polygon, MultiPolygon, shape
from shapely.ops import nearest_points

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

def calculate(job_id, job):
    print('calculating...')
    print(job)
    constraints = job.get('constraints')
    if job.get('cluster_location'):
        return location_based_scheduling(job)  # tuple of (negative|positive, cluster|negative_description)
    else:
        if len(constraints) >= 1:
            # Filter clusters that are not close to area specified in SLA constriants
            clusters = filter_clusters_based_on_worker_locations(constraints)
        else:
            clusters = mongo_find_all_active_clusters()
        return greedy_load_balanced_algorithm(job=job, filtered_clusters=clusters)


def location_based_scheduling(job):
    requested_location = job.get('cluster_location')
    cluster = mongo_find_cluster_by_location(requested_location)  # can return None

    if cluster is not None:  # cluster found by location exists
        if is_cluster_active(cluster):
            print('Cluster is active')

            job_required_cpu_cores = job.get('requirements').get('cpu')
            job_required_memory = job.get('requirements').get('memory')

            cluster_cores_available = cluster.get('total_cpu_cores')
            cluster_memory_available = cluster.get('memory_in_mb')

            if cluster_cores_available >= job_required_cpu_cores and cluster_memory_available >= job_required_memory:
                return 'positive', cluster
            else:
                return 'negative', 'TargetClusterNoCapacity'
        else:
            return 'negative', 'TargetClusterNotActive'
    else:
        return 'negative', 'TargetClusterNotFound'  # no cluster Found


def first_fit_algorithm(job):
    """Which of the clusters fits the Qos of the deployment file as the first"""
    active_clusters = mongo_find_all_active_clusters()

    print('active_clusters: ')
    for cluster in active_clusters:
        print(cluster)

        available_cpu = cluster.get('current_cpu_cores_free')
        available_memory = cluster.get('current_free_memory_in_MB')
        node_info = cluster.get('node_info')
        # technology = node_info.get('technology')
        virtualization = node_info.get('virtualization')

        # job_req = job.get('requirements')
        job_req = {'memory': job.get('memory'), 'vcpus': job.get('vcpus')}

        if available_cpu >= job_req.get('vcpus') and available_memory >= job_req.get('memory') and job.get(
                'virtualization') in virtualization:
            return 'positive', cluster

    # no cluster found
    return 'negative', 'NoActiveClusterWithCapacity'


def greedy_load_balanced_algorithm(job, filtered_clusters):
    """Which of the clusters have the most capacity for a given job"""

    # job_req = job.get('requirements')
    job_req = {'memory': job.get('memory'), 'vcpus': job.get('vcpus')}

    qualified_clusters = []

    for cluster in filtered_clusters:
        available_cpu = float(cluster.get('total_cpu_cores'))
        available_memory = float(cluster.get('memory_in_mb'))

        if available_cpu >= job_req.get('vcpus') and available_memory >= job_req.get('memory'):
            qualified_clusters.append(cluster)

    target_cluster = None
    target_cpu = 0
    target_mem = 0

    # return if no qualified clusters found
    if not qualified_clusters:
        return 'negative', 'NoActiveClusterWithCapacity'

    # return the cluster with the most cpu+ram
    for cluster in qualified_clusters:
        cpu = float(cluster.get('total_cpu_cores'))
        mem = float(cluster.get('memory_in_mb'))

        if cpu >= target_cpu and mem >= target_mem:
            target_cpu = cpu
            target_mem = mem
            target_cluster = cluster

    return 'positive', target_cluster


def same_cluster_replication(job_obj, cluster_obj, replicas):
    job_description = job_obj.get('file_content')

    job_required_cpu_cores = job_description.get('requirements').get('cpu')
    job_required_memory = job_description.get('requirements').get('memory')

    cluster_cores_available = cluster_obj.get('total_cpu_cores')
    cluster_memory_available = cluster_obj.get('memory_in_mb')


def filter_clusters_based_on_worker_locations(constraints):
    active_clusters = mongo_find_all_active_clusters()
    constraint_locations = {}
    for constraint in constraints:
        constraint_type = constraint["type"]
        if constraint_type == "geo":
            lat, long = constraint["location"].split(",")
            loc = Point(float(lat), float(long))
            threshold = constraint["threshold"]
            area = loc.buffer(threshold)
            constraint_locations.setdefault("geo", []).append(area)
        elif constraint_type == "latency":
            area = constraint["area"]
            if area in AREAS:
                constraint_locations.setdefault("latency", []).append(AREAS[area])
            else:
                raise f"Area not supported: {area}"
        else:
            raise f"Unsupported latency type: {constraint_type}"

    filtered_clusters = []
    for c in active_clusters:
        print(f"Cluster: {c}")
        feasible = True
        worker_area = shape(c.get("worker_groups"))

        # 1. Filter based on "geo" constraint, i.e., do not consider clusters that do not have nodes in every area
        if "geo" in constraint_locations:
            for area in constraint_locations["geo"]:
                # If the cluster does not intersects with an constraint area continue with next cluster
                if not cluster_intersects_area(worker_area, area):
                    unfeasible = True
                    break
        # 2. Filter based on "latency" constraint
        elif "latency" in constraint_locations:
            # TODO: How close should the cluster be to the specified area?
            pass
        if feasible:
            filtered_clusters.append(c)

    return filtered_clusters


def cluster_intersects_area(cluster, area):
    """
    Checks whether the 'user' is located within the cluster or its boundaries. Since shapely is coordinate-agnostic it
    will handle geographic coordinates expressed in latitudes and longitudes exactly the same way as coordinates on a
    Cartesian plane. But on a sphere the behavior is different and angles are not constant along a geodesic.
    For that reason we do a small distance correction here.
    """
    return cluster.intersects(area) or cluster.distance(area) < 1e-5

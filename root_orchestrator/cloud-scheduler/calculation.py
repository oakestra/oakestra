import time

from mongodb_client import mongo_find_cluster_by_location, is_cluster_active, mongo_find_all_active_clusters
import geopy.distance
from shapely.geometry import Point, Polygon, MultiPolygon, shape
from shapely.ops import nearest_points

def calculate(job_id, job, deployment_request_coords):
    print('calculating...')
    print(job)
    if job.get('cluster_location'):
        return location_based_scheduling(job)  # tuple of (negative|positive, cluster|negative_description)
    else:
        # TODO: clarify if it even makes sense to filter out clusters that are not close to location of request. Wouldn't
        #  it make more sense that we check if any constraints are speciefied and then filter out clusters that do not have
        #  nodes in the specified locations?
        #  Filter clusters based on location of the worker nodes belonging to the cluster
        closest_clusters = filter_clusters_based_on_worker_locations(deployment_request_coords)
        return greedy_load_balanced_algorithm(job=job, closest_clusters=closest_clusters)


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


def greedy_load_balanced_algorithm(job, closest_clusters):
    """Which of the clusters have the most capacity for a given job"""

    # job_req = job.get('requirements')
    job_req = {'memory': job.get('memory'), 'vcpus': job.get('vcpus')}

    qualified_clusters = []

    for cluster_name, cluster_dist_dict in closest_clusters.items():
        print(cluster_name)
        cluster = cluster_dist_dict['cluster']
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


def filter_clusters_based_on_worker_locations(user_coords):
    active_clusters = mongo_find_all_active_clusters()
    lat = user_coords['lat']
    long = user_coords['long']
    user = Point(float(lat), float(long))

    cluster_user_dists = {}
    for c in active_clusters:
        cluster_name = c.get('cluster_name')
        geo = shape(c.get('worker_groups'))
        # If the developer is located within the cluster add it to list of clusters containing the user
        if user_in_cluster(user, geo):
            cluster_user_dists[cluster_name] = {'cluster': c, 'dist': -1}
        # Otherwise calculate shortest distance between user and cluster and add the distance to the list
        else:
            p1, _ = nearest_points(geo, user)
            user_cluster_dist = geopy.distance.distance((p1.x, p1.y), (user.x, user.y)).km
            cluster_user_dists[cluster_name] = {'cluster': c, 'dist': user_cluster_dist}

    return dict(sorted(cluster_user_dists.items(), key=lambda item: item[1]['dist']))

# TODO: check if also needed in system manager
def user_in_cluster(user, cluster):
    """
    Checks whether the 'user' is located within the cluster or its boundaries. Since shapely is coordinate-agnostic it
    will handle geographic coordinates expressed in latitudes and longitudes exactly the same way as coordinates on a
    Cartesian plane. But on a sphere the behavior is different and angles are not constant along a geodesic.
    For that reason we do a small distance correction here.
    """
    return True if cluster.intersects(user) or user.within(cluster) or cluster.distance(user) < 1e-5 else False

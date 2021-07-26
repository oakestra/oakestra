import time

from mongodb_client import mongo_find_cluster_by_location, is_cluster_active, mongo_find_all_active_clusters


def calculate(job_id, job):
    print('calculating...')

    if job.get('cluster_location'):
        return location_based_scheduling(job)  # tuple of (negative|positive, cluster|negative_description)
    else:
        return greedy_load_balanced_algorithm(job=job)


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
        technology = node_info.get('technology')

        job_req = job.get('requirements')
        if available_cpu >= job_req.get('cpu') and available_memory >= job_req.get('memory') and job.get(
                'image_runtime') in technology:
            return 'positive', cluster

    # no cluster found
    return 'negative', 'NoActiveClusterWithCapacity'


def greedy_load_balanced_algorithm(job):
    """Which of the clusters have the most capacity for a given job"""

    job_req = job.get('requirements')

    active_clusters = mongo_find_all_active_clusters()
    qualified_clusters = []

    for cluster in active_clusters:
        print(cluster)
        available_cpu = float(cluster.get('total_cpu_cores'))
        available_memory = float(cluster.get('memory_in_mb'))

        if available_cpu >= job_req.get('cpu') and available_memory >= job_req.get('memory'):
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

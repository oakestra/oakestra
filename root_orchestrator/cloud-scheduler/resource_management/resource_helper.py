from _datetime import datetime

CLUSTERS_FRESHNESS_INTERVAL = 30

def is_cluster_active(cluster):
    timestamp_now = datetime.now().timestamp()
    last_modified_cluster = cluster.get('last_modified_timestamp')
    return last_modified_cluster >= timestamp_now - CLUSTERS_FRESHNESS_INTERVAL

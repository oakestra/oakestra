from bson.objectid import ObjectId
from _datetime import datetime

CLUSTERS_FRESHNESS_INTERVAL = 30

def get_active_filter():
    now_timestamp = datetime.now().timestamp()
    return {'$gt': now_timestamp - CLUSTERS_FRESHNESS_INTERVAL}

def build_filter(query):
    filter = query
    if filter.get('active'):
        filter['last_modified_timestamp'] = get_active_filter()
    if filter.get('cluster_id'):
        filter['_id'] = ObjectId(filter.get('cluster_id'))
        
    filter.pop('cluster_id', None)
    filter.pop('job_id', None)
    filter.pop('active', None)
    return filter

def is_cluster_active(cluster):
    timestamp_now = datetime.now().timestamp()
    last_modified_cluster = cluster.get('last_modified_timestamp')
    return last_modified_cluster >= timestamp_now - CLUSTERS_FRESHNESS_INTERVAL
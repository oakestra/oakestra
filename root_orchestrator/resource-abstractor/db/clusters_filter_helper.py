from bson.objectid import ObjectId
from _datetime import datetime

CLUSTERS_FRESHNESS_INTERVAL = 30

def build_filter(query):
    filter = query
    if filter.get('active'):
        now_timestamp = datetime.now().timestamp()
        filter['last_modified_timestamp'] = {'$gt': now_timestamp - CLUSTERS_FRESHNESS_INTERVAL}
    if filter.get('cluster_id'):
        filter['_id'] = ObjectId(filter.get('cluster_id'))
        
    filter.pop('cluster_id', None)
    filter.pop('job_id', None)
    filter.pop('active', None)
    return filter
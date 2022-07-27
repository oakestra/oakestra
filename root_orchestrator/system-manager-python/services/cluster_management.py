import traceback

from ext_requests.cluster_db import *
from services.service_management import delete_service


def register_cluster(clusters, userid):
    for cluster in clusters['clusters']:

        if "action" in cluster:
            del cluster['action']
        if "_id" in cluster:
            del cluster['_id']
        cluster['userId'] = userid
        microservices = cluster.get('microservices')
        cluster['microservices'] = []
        cluster_id = mongo_add_cluster(cluster)

        # register microservices as well if any
        if cluster_id:
            if len(microservices) > 0:
                try:
                    cluster['microservices'] = microservices
                    cluster['clusterID'] = cluster_id
                    result, status = create_services_of_cluster(
                        userid,
                        {
                            'sla_version': clusters['sla_version'],
                            'customerID': userid,
                            'clusters': [cluster]
                        }
                    )
                    if status != 200:
                        delete_cluster(cluster_id, userid)
                        return result, status
                except Exception as e:
                    print(traceback.format_exc())
                    delete_cluster(cluster_id, userid)
                    return {'message': 'error during the registration of the microservices'}, 500

    return mongo_get_clusters_of_user(userid), 200



'''TODO: All functions above to be implemented properly

def update_cluster(clusterid, userid, fields):
    # TODO: fields validation before update
    return mongo_update_cluster(clusterid, userid, fields)


def delete_cluster(clusterid, userid):
    cluster = get_user_cluster(userid, clusterid)
    return mongo_delete_cluster(clusterid, userid)


def users_clusters(userid):
    return mongo_get_clusters_of_user(userid)


def all_clusters():
    return mongo_get_all_clusters()


def get_user_cluster(userid, clusterid):
    return mongo_find_cluster_by_id(clusterid, userid)'''
import traceback

from ext_requests.cluster_db import *
from services.service_management import delete_service


def register_cluster(clusters, userid):
    clusters['userId'] = userid
    return mongo_add_cluster(clusters)


'''TODO: All functions above might be implemented properly
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

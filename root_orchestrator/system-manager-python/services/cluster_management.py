import traceback

from ext_requests.cluster_db import *
from services.service_management import delete_service


def register_cluster(clusters, userid):
    clusters['userId'] = userid
    return mongo_add_cluster(clusters)


def update_cluster(cluster_id, userid, fields):
    # TODO: fields validation before update
    fields['userId'] = userid
    return mongo_update_cluster_information(cluster_id, fields)


def delete_cluster(cluster_id, userid):
    cluster = get_user_cluster(userid, cluster_id)
    return mongo_delete_cluster(cluster_id, userid)


def users_clusters(userid):
    return mongo_get_clusters_of_user(userid)


def all_clusters():
    return mongo_get_all_clusters()


def get_user_cluster(userid, cluster_id):
    return mongo_find_cluster_by_id(cluster_id)

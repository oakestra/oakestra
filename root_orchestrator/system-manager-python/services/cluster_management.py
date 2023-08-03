import logging
import traceback
from random import randint

from ext_requests.cluster_db import *
from roles import securityUtils
from datetime import datetime, timedelta


def valid_cluster_requirements(cluster):
    if len(cluster.get('cluster_name')) > 15 or len(cluster.get('cluster_name')) < 1:
        return False
    return True


def register_cluster(cluster, username):
    if "action" in cluster:
        del cluster['action']
    if "_id" in cluster:
        del cluster['_id']
    if username is None:
        return {"message": "Please log in with your credentials"}, 403
    if cluster is None:
        return {"message": "No cluster data provided"}, 403
    if not valid_cluster_requirements(cluster):
        return {'message': 'Cluster name is not in the valid format'}, 422
    cl_ob = mongo_find_by_name_and_location(cluster)
    if cl_ob is not None and not cl_ob['pairing_complete']:
        return {'message': 'There is another cluster with the same exact location trying to pair'}, 422

    cluster['user_name'] = username
    cluster['pairing_complete'] = False
    cluster_id = mongo_add_cluster(cluster)
    if cluster_id == "":
        logging.log(level=logging.ERROR, msg="Invalid input")
        return {}

    additional_claims = {"iat": datetime.now().timestamp(),
                         "aud": "addClusterAPI",
                         "sub": cluster['user_name'], # BUG: Overrides cluster_id from identity
                         "clusterName": cluster['cluster_name'],
                         "num": str(randint(0, 99999999))}

    token = securityUtils.create_jwt_pairing_key_cluster(cluster_id, timedelta(hours=5), additional_claims)

    # cluster['pairing_key'] = token --> we check the validity of tokens by the decoding call
    #mongo_update_pairing_key(userid, cluster_id, cluster)
    return {"pairing_key": token}


def update_cluster(cluster_id, userid, fields):
    # TODO: fields validation before update
    fields['userId'] = userid
    return mongo_update_cluster_information(cluster_id, fields)


def delete_cluster(cluster_id):
    mongo_delete_cluster(cluster_id)


def users_clusters(userid):
    return mongo_get_clusters_of_user(userid)


def all_clusters():
    return mongo_get_all_clusters()


def get_user_cluster(userid, cluster_id):
    return mongo_find_cluster_by_id(cluster_id)

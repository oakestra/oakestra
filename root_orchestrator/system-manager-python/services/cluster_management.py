import logging
import traceback

from ext_requests.cluster_db import *
from roles import securityUtils
from services.service_management import delete_service


def valid_cluster_requirements(cluster):
    if len(cluster.get('cluster_name')) > 10 or len(cluster.get('cluster_name')) < 1:
        return False
    return True


def register_cluster(cluster, userid):
    if "action" in cluster:
        del cluster['action']
    if "_id" in cluster:
        del cluster['_id']
    if userid is None:
        return {"message": "Please log in with your credentials"}, 403
    if cluster is None:
        return {"message": "No cluster data provided"}, 403
    if not valid_cluster_requirements(cluster):
        return {'message': 'Cluster name is not in the valid format'}, 422

    cluster['userId'] = userid
    cluster_id = mongo_add_cluster(cluster)
    if cluster_id == "":
        logging.log(level=logging.ERROR, msg="Invalid input")
        return {}

    cluster_of_user_identifier = userid + cluster_id
    # TODO: We have to define the expiration date for the access token
    secret_key = securityUtils.create_jwt_secret_key_cluster(identity=cluster_of_user_identifier)
    # TODO: We have to define the refresh token for the pairing key

    cluster['pairing_key'] = secret_key
    mongo_update_cluster(userid, cluster_id, cluster)

    return {"secret key": secret_key}


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

import logging
import traceback

from ext_requests.cluster_db import *
from roles import securityUtils
from datetime import datetime, timedelta
from services.service_management import delete_service


def valid_cluster_requirements(cluster):
    if len(cluster.get('cluster_name')) > 15 or len(cluster.get('cluster_name')) < 1:
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
    cl_ob = mongo_find_by_name_and_location(cluster)
    if cl_ob is not None and not cl_ob['pairing_complete']:
        return {'message': 'There is another cluster with the same exact location trying to pair'}, 422

    cluster['userId'] = userid
    cluster['pairing_complete'] = False
    cluster_id = mongo_add_cluster(cluster)
    if cluster_id == "":
        logging.log(level=logging.ERROR, msg="Invalid input")
        return {}

    # change the Bearer token into a Proof of Possession token (a PoP token) by adding a cnf claim a confirmation claim

    # add the additional claims that must include: expiration date, secret_key2 (the one that will be returned to the
    # front, the alg, identity and some data of the cluster - time that has been required to be added)

    # sec_key = current_app.config["JWT_SECRET_KEY"]

    dt_string = datetime.now().strftime("%d/%m/%Y %H:%M:%S")  # example: 25/06/2021 07:58:56
    additional_claims = {"iat": dt_string, "aud": "addClusterAPI", "userid": userid}
    expiry_date = timedelta(hours=5)

    token = securityUtils.create_jwt_secret_key_cluster(cluster_id, expiry_date, additional_claims)

    cluster['pairing_key'] = token
    mongo_update_pairing_key(userid, cluster_id, cluster)
    return {"secret_key": token}


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

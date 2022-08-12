from bson import json_util
from flask.views import MethodView
from flask import request
from flask_jwt_extended import get_jwt_identity, jwt_required
from flask_smorest import Blueprint, abort

from ext_requests.cluster_requests import cluster_request_to_delete_job_by_ip
from ext_requests.apps_db import mongo_update_job_status
from services.cluster_management import *
from ext_requests.cluster_db import *
import traceback

clustersblp = Blueprint(
    'Clusters', 'cluster management', url_prefix='/api/clusters'
)

clusterinfo = Blueprint(
    'Clusterinfo', 'cluster information', url_prefix='/api/information'
)

clusterblp = Blueprint(
    'Cluster operations', 'cluster operations', url_prefix='/api/cluster'
)

cluster_info_schema = {
    "type": "object",
    "properties": {
        "cpu_percent": {"type": "string"},
        "cpu_cores": {"type": "string"},
        "gpu_cores": {"type": "string"},
        "gpu_percent": {"type": "string"},
        "cumulative_memory_in_mb": {"type": "string"},
        "number_of_nodes": {"type": "string"},
        "virtualization": {"type": "array", "items": {"type": "string"}},
        "more": {"type": "object"},
        "worker_groups": {"type": "string"},
        "jobs": {
            "type": "array",
            "items": {
                "type": "object",
                "properties": {
                    "system_job_id": {"type": "string"},
                    "status": {"type": "string"},
                    "instance_list": {
                        "type": "array",
                        "items": {
                            "type": "object",
                            "properties": {
                                "instance_number": {"type": "string"},
                                "status": {"type": "string"},
                                "status_detail": {"type": "string"},
                                "publicip": {"type": "string"}
                            }
                        }
                    },
                }
            }
        },
    }
}

'''Base model to add a cluster as a single simple node'''
cluster_op_schema = {
    "type": "object",
    "properties": {
        "cluster_name": {"type": "string"},
        "cluster_latitude": {"type": "string"},
        "cluster_longitude": {"type": "string"},
        "cluster_radius": {"type": "string"},
        "user_name": {"type": "string"}
    }
}


@clustersblp.route('/')
class ClustersController(MethodView):

    def get(self, *args, **kwargs):
        return json_util.dumps(mongo_get_all_clusters())


@clustersblp.route('/<userid>')
class ClustersController(MethodView):

    def get(self, userid, *args, **kwargs):
        return json_util.dumps(mongo_get_clusters_of_user(userid))


@clustersblp.route('/active')
class ActiveClustersController(MethodView):

    def get(self, *args, **kwargs):
        return json_util.dumps(mongo_find_all_active_clusters())


@clusterinfo.route('/<clusterid>')
class ClusterController(MethodView):

    @clusterinfo.arguments(schema=cluster_info_schema, location="json", validate=False, unknown=True)
    def post(self, *args, **kwargs):
        data = request.json
        mongo_update_cluster_information(kwargs['clusterid'], data)
        jobs = data.get('jobs')
        for j in jobs:
            result = mongo_update_job_status(
                job_id=j.get('system_job_id'),
                status=j.get('status'),
                status_detail=j.get('status_detail'),
                instances=j.get('instance_list'))
            if result is None:
                # cluster has outdated jobs, ask to undeploy
                cluster_request_to_delete_job_by_ip(j.get('system_job_id'), -1, request.remote_addr)

        return 'ok'


@clusterblp.route('/add')
class ClusterController(MethodView):

    @clusterblp.arguments(schema=cluster_op_schema, location="json", validate=False, unknown=True)
    @clusterblp.response(200, content_type="application/json")
    @jwt_required()
    def post(self, args, *kwargs):
        data = request.get_json()
        current_user = get_jwt_identity()
        try:
            resp = register_cluster(data, current_user)
            return resp
        except Exception as e:
            traceback.print_exc()
            abort(401, {"message": str(e)})


@clusterblp.route('/<cluster_id>')
class ApplicationController(MethodView):

    @clusterblp.response(200, content_type="application/json")
    @jwt_required()
    def get(self, cluster_id, *args, **kwargs):
        try:
            current_user = get_jwt_identity()
            return json_util.dumps(get_user_cluster(current_user, cluster_id))
        except Exception as e:
            return abort(404, {"message": e})

    # TODO: treat exception
    @jwt_required()
    def delete(self, cluster_id):
        delete_cluster(cluster_id)
        return {"message": "Cluster Deleted"}

    @jwt_required()
    def put(self, cluster_id, *args, **kwargs):
        print(request.get_json())
        try:
            current_user = get_jwt_identity()
            update_cluster(cluster_id, current_user, request.get_json())
            return {"message": "Cluster is updated"}
        except ConnectionError as e:
            abort(404, {"message": e})

import logging
import traceback
import json

from bson import json_util
from flask.views import MethodView
from flask import request
from flask_smorest import Blueprint, Api, abort

from ext_requests.cluster_db import mongo_get_all_clusters, mongo_find_all_active_clusters, \
    mongo_update_cluster_information
from services.instance_management import instance_scale_up_scheduled_handler

clustersbp = Blueprint(
    'Clusters', 'cluster management', url_prefix='/api/clusters'
)

clusterinfo = Blueprint(
    'Clusterinfo', 'cluster informations', url_prefix='/api/information'
)

cluster_info_schema = {
    "type": "object",
    "properties": {
        "cpu_percent": {"type": "string"},
        "cpu_cores": {"type": "string"},
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
                            }
                        }
                    },
                }
            }
        },
    }
}


@clustersbp.route('/')
class ClustersController(MethodView):

    def get(self, *args, **kwargs):
        return json_util.dumps(mongo_get_all_clusters())


@clustersbp.route('/active')
class ActiveClustersController(MethodView):

    def get(self, *args, **kwargs):
        return json_util.dumps(mongo_find_all_active_clusters())


@clusterinfo.route('/<clusterid>')
class ClusterController(MethodView):

    @clusterinfo.arguments(schema=cluster_info_schema, location="json", validate=False, unknown=True)
    def post(self, clusterid, *args, **kwargs):
        data = request.json
        mongo_update_cluster_information(clusterid, data)

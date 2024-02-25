from ext_requests.apps_db import mongo_update_job_status
from ext_requests.cluster_requests import cluster_request_to_delete_job_by_ip
from flask import request
from flask.views import MethodView
from flask_smorest import Blueprint
from ras_client import cluster_operations

clustersbp = Blueprint("Clusters", "cluster management", url_prefix="/api/clusters")
clusterinfo = Blueprint("Clusterinfo", "cluster informations", url_prefix="/api/information")

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
                                "publicip": {"type": "string"},
                            },
                        },
                    },
                },
            },
        },
    },
}


@clustersbp.route("/")
class ClustersController(MethodView):
    def get(self, *args, **kwargs):
        return cluster_operations.get_all_clusters()


@clustersbp.route("/active")
class ActiveClustersController(MethodView):
    def get(self, *args, **kwargs):
        return cluster_operations.get_resources(active=True)


@clusterinfo.route("/<clusterid>")
class ClusterController(MethodView):
    @clusterinfo.arguments(
        schema=cluster_info_schema, location="json", validate=False, unknown=True
    )
    def post(self, *args, **kwargs):
        data = request.json
        cluster_id = kwargs["clusterid"]
        cluster_operations.update_cluster_information(cluster_id, data)

        # TODO: fire an event to react to the cluster update, and move this logic somewhere else.
        jobs = data.get("jobs")
        for j in jobs:
            result = mongo_update_job_status(
                job_id=j.get("system_job_id"),
                status=j.get("status"),
                status_detail=j.get("status_detail"),
                instances=j.get("instance_list"),
            )
            if result is None:
                # cluster has outdated jobs, ask to undeploy
                cluster_request_to_delete_job_by_ip(j.get("system_job_id"), -1, request.remote_addr)

        return "ok"

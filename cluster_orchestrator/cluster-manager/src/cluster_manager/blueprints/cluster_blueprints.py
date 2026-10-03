import logging

from bson import json_util
from flask import Response
from flask.views import MethodView
from flask_smorest import Blueprint

from ..app_config import CONFIG, RUNTIME_CONFIG

logger = logging.getLogger("cluster_manager")

clusterblp = Blueprint(
    "Cluster operations",
    "cluster",
    url_prefix="/api/cluster",
    description="Cluster status operations",
)


@clusterblp.route("/status")
class ClusterStatusController(MethodView):
    @clusterblp.response(
        200,
        {},
        content_type="application/json",
    )
    def get(self):
        logger.debug("Incoming Request GET /api/cluster/status")
        response = {
            "cluster_name": CONFIG.cluster_name,
            "cluster_id": RUNTIME_CONFIG.assigned_cluster_id,
            "connected_to_root": RUNTIME_CONFIG.assigned_cluster_id is not None,
        }
        return Response(json_util.dumps(response), mimetype="application/json")

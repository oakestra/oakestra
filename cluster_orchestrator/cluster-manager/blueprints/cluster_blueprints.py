import logging

import cluster_manager as cm
from bson import json_util
from flask import Response
from flask.views import MethodView
from flask_smorest import Blueprint

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
        connected = cm.MY_ASSIGNED_CLUSTER_ID is not None
        response = {
            "cluster_name": cm.MY_CHOSEN_CLUSTER_NAME,
            "cluster_id": cm.MY_ASSIGNED_CLUSTER_ID,
            "connected_to_root": connected,
        }
        return Response(json_util.dumps(response), mimetype="application/json")

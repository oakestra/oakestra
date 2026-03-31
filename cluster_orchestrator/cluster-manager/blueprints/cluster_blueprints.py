import logging

import config
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
        response = {
            "cluster_name": config.MY_CHOSEN_CLUSTER_NAME,
            "cluster_id": config.MY_ASSIGNED_CLUSTER_ID,
            "connected_to_root": config.MY_ASSIGNED_CLUSTER_ID is not None,
        }
        return Response(json_util.dumps(response), mimetype="application/json")

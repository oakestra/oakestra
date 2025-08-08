import logging
import os

from bson import json_util
from clients.mongodb_client import mongo_upsert_node
from flask import request, Response
from flask.views import MethodView
from flask_smorest import Blueprint, abort


# ........ Functions for job management ...............#
# ......................................................#

workerblp = Blueprint(
    "Service operations",
    "service",
    url_prefix="/api/node",
    description="Node registration operations",
)


@workerblp.route("/register")
class ServiceController(MethodView):
    @workerblp.response(
        200,
        {},
        content_type="application/json",
    )
    def post(self):
        logging.info("Incoming Request /api/node/register - to register node")
        data = request.json  # get POST body
        data.get("token")  # registration_token
        # TODO(GB): check and generate tokens
        client_id = mongo_upsert_node({"ip": request.remote_addr, "node_info": data})
        if not client_id:
            logging.error("Failed to register node, client_id is None")
            abort(500, "Failed to register node")
        response = {
            "id": str(client_id),
            "MQTT_BROKER_PORT": os.environ.get("MQTT_BROKER_PORT"),
        }
        return Response(json_util.dumps(response), mimetype="application/json")

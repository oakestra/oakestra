import logging
import os

from bson import json_util
from flask import Response, request
from flask.views import MethodView
from flask_smorest import Blueprint, abort
from resource_abstractor_client import candidate_operations

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
        print("Incoming Request /api/node/register - to register node")
        data = request.json  # get POST body
        data.get("token")  # registration_token
        # TODO(GB): check and generate tokens   
        print("Data: ", data.get("system_info"))
        data["candidate_name"] = data.get("full_stats", {}).get("hostname", "")
        worker = candidate_operations.create_candidate(data)
        if worker is None:
            logging.error("Failed to register node")
            abort(500, "Failed to register node")

        worker_id = str(worker["_id"])
        response = {
            "id": str(worker_id),
            "MQTT_BROKER_PORT": os.environ.get("MQTT_BROKER_PORT"),
        }
        return Response(json_util.dumps(response), mimetype="application/json")

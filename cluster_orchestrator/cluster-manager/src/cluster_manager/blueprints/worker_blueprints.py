from bson import json_util
from flask import Response, request
from flask.views import MethodView
from flask_smorest import Blueprint, abort
from oakestra_logging import get_logger
from resource_abstractor_client import candidate_operations

from ..app_config import CONFIG

logger = get_logger(__name__)

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
        logger.debug(
            "Received worker registration request",
            event_name="worker.registration.received",
        )
        data = request.json  # get POST body
        data.get("token")  # registration_token
        # TODO(GB): check and generate tokens
        data["candidate_name"] = data.get("host", "")
        logger.debug(
            "Prepared worker candidate name",
            event_name="worker.registration.candidate_prepared",
            candidate_name=data.get("candidate_name"),
        )
        worker = candidate_operations.create_candidate(data)
        if worker is None:
            logger.error(
                "Worker registration failed",
                event_name="worker.registration.failed",
            )
            abort(500, message="Failed to register node")

        worker_id = str(worker["_id"])
        response = {
            "id": str(worker_id),
            # NodeEngine expects a string in the registration response.
            "MQTT_BROKER_PORT": str(CONFIG.mqtt_broker_port),
        }
        return Response(json_util.dumps(response), mimetype="application/json")

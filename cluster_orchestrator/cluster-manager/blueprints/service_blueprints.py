import logging
import traceback

from bson import json_util
from clients import job_management
from clients.job_management import deploy_job
from clients.mqtt_client import mqtt_publish_edge_deploy
from ext_requests.network_manager_requests import network_notify_deployment
from flask import Response, request
from flask.views import MethodView
from flask_smorest import Blueprint, abort
from oakestra_utils.types.statuses import (
    PositiveSchedulingStatus,
    convert_to_status,
)
from resource_abstractor_client import job_operations

logger = logging.getLogger("cluster_manager")

# ........ Functions for job management ...............#
# ......................................................#

serviceblp = Blueprint(
    "Multiple services operations",
    "services",
    url_prefix="/api/service",
    description="Operations on services",
)

schedulingblp = Blueprint(
    "Scheduling results",
    "service",
    url_prefix="/api/result",
    description="Scheduling results operations",
)


@serviceblp.route("/<job_id>/<instance_number>")
class ServiceController(MethodView):
    @serviceblp.response(
        200,
        {"status": "ok"},
        content_type="application/json",
    )
    def post(self, job_id, instance_number):
        job = request.json  # contains job_id and job_description

        try:
            logger.info(f"Received deployment request for instance {instance_number} of {job}")
            deploy_job(job, instance_number)
        except Exception as e:
            logger.error(f"Deployment Failed: {e}")
            logger.error(f"{traceback.format_exc()}")
            abort(500, "Failed to deploy service")

        return Response(json_util.dumps({"status": "ok"}), mimetype="application/json")

    @serviceblp.response(
        200,
        {"status": "ok"},
        content_type="application/json",
    )
    def delete(self, job_id, instance_number):
        """
        find service in db and ask corresponding worker to delete task,
        instance_number -1 undeploy all known instances
        """
        logger.info("Incoming Request /api/delete/ - to delete task...")

        try:
            job_management.delete_job_instance(job_id, int(instance_number), erase=True)
        except Exception as e:
            logger.error(f"Failed to delete service {job_id}: {e}")
            logger.error(f"{traceback.format_exc()}")
            abort(500, "Failed to delete service")

        return Response(json_util.dumps({"status": "ok"}), mimetype="application/json")


@schedulingblp.route("/deploy")
class SchedulingController(MethodView):
    @serviceblp.response(
        200,
        {},
        content_type="application/json",
    )
    def post(self):
        data = request.get_json()
        logger.debug(data)
        id = data.get("job_id").split("/")
        job_id = id[0]
        instance_number = id[1]
        node_id = data.get("candidate_id")
        logger.info(
            "Received scheduling result for job "
            + job_id
            + " instance "
            + instance_number
            + ". Result: "
            + node_id
        )
        if node_id is None:
            # scheduling failed
            status = data.get("status")
            job_operations.update_job_status(job_id, convert_to_status(status))
            return Response(json_util.dumps({"status": "ok"}), mimetype="application/json")

        # update job instance
        job_management.update_instance_node(job_id, int(instance_number), node_id)
        job_management.update_status(
            job_id, int(instance_number), PositiveSchedulingStatus.NODE_SCHEDULED.value
        )

        job = job_operations.get_job_by_id(job_id)
        if job is None:
            logger.error("Job " + job_id + " has been deleted")
            return Response(
                json_util.dumps({"status": "job_not_found"}), mimetype="application/json"
            )

        # update network component
        network_notify_deployment(job_id, job)

        # publish job
        mqtt_publish_edge_deploy(node_id, job, instance_number)
        return Response(json_util.dumps({"status": "ok"}), mimetype="application/json")

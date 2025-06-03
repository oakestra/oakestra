import logging

from bson import json_util
from flask import request, Response
from flask.views import MethodView
from flask_smorest import Blueprint, abort
from ext_requests.network_plugin_requests import network_notify_deployment
from clients.mqtt_client import mqtt_publish_edge_deploy
import services.service_operations as service_operations
from oakestra_utils.types.statuses import NegativeSchedulingStatus, PositiveSchedulingStatus
from clients.mongodb_client import (
    mongo_find_job_by_system_id,
    mongo_update_job_status,
)


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


@serviceblp.route("/<system_job_id>/<instance_number>")
class ServiceController(MethodView):
    @serviceblp.response(
        200,
        {"status": "ok"},
        content_type="application/json",
    )
    def post(self, system_job_id, instance_number):
        logging.info("Incoming Request /api/deploy")
        job = request.json  # contains job_id and job_description

        try:
            service_operations.deploy_service(job, system_job_id, instance_number)
        except Exception:
            abort(500, "Failed to deploy service")

        return Response(json_util.dumps({"status": "ok"}), mimetype='application/json')

    @serviceblp.response(
        200,
        {"status": "ok"},
        content_type="application/json",
    )
    def delete(self, system_job_id, instance_number):
        """
        find service in db and ask corresponding worker to delete task,
        instance_number -1 undeploy all known instances
        """
        logging.info("Incoming Request /api/delete/ - to delete task...")

        try:
            service_operations.delete_service(system_job_id, instance_number)
        except Exception:
            abort(500, "Failed to delete service")

        return Response(json_util.dumps({"status": "ok"}), mimetype='application/json')


@schedulingblp.route("/<system_job_id>/<instance_number>")
class SchedulingController(MethodView):
    @serviceblp.response(
        200,
        {},
        content_type="application/json",
    )
    def post(self, system_job_id, instance_number):
        logging.info("Incoming Request /api/result - received cluster_scheduler result")
        data = request.json  # get POST body
        logging.info(data)

        if data.get("found", False):
            resulting_node_id = data.get("node").get("_id")
            mongo_update_job_status(
                system_job_id=system_job_id,
                instance_number=instance_number,
                node=data.get("node"),
                status=PositiveSchedulingStatus.NODE_SCHEDULED,
            )
            job = mongo_find_job_by_system_id(system_job_id)

            # Inform network plugin about the deployment
            network_notify_deployment(str(job["system_job_id"]), job)

            # Publish job
            mqtt_publish_edge_deploy(resulting_node_id, job, instance_number)
        else:
            mongo_update_job_status(
                instance_number=instance_number,
                system_job_id=system_job_id,
                node=None,
                status=NegativeSchedulingStatus.NO_WORKER_CAPACITY,
            )
        return Response(json_util.dumps({"status": "ok"}), mimetype='application/json')

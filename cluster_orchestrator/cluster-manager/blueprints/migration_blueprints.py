import logging

from bson import json_util
from flask import request, Response
from flask.views import MethodView
from flask_smorest import Blueprint, abort
import services.service_operations as service_operations
from oakestra_utils.types.statuses import DeploymentStatus
from clients.mongodb_client import (
    mongo_find_job_by_system_id_and_instance,
    mongo_find_node_by_id,
    mongo_find_node_by_name,
    mongo_update_job_status,
)
import threading
from marshmallow import Schema, fields

# ........ Functions for job management ...............#
# ......................................................#

migrationblp = Blueprint(
    "Service migration operations",
    "services",
    url_prefix="/api/migration",
    description="Operations on services",
)


class MigrationRequestSchema(Schema):
    system_job_id = fields.Str(required=True)
    instance_number = fields.Int(required=True)
    target_node_id = fields.Str(required=True)


migration_lock = threading.Lock()


@migrationblp.route("/")
class ServiceController(MethodView):
    @migrationblp.arguments(
        schema=MigrationRequestSchema, location="json", validate=False, unknown=True
    )
    @migrationblp.response(
        200,
        content_type="application/json",
    )
    def post(self, *args, **kwargs):
        logging.info("Incoming Request /api/migration")
        migrationRequest = request.json  # contains job_id and job_description
        instance_number = migrationRequest.get("instance_number")
        system_job_id = migrationRequest.get("system_job_id")

        # Validate the migration request
        if not migrationRequest or not isinstance(migrationRequest, dict):
            abort(400, "Invalid migration request format")
        if "system_job_id" not in migrationRequest or "instance_number" not in migrationRequest:
            abort(400, "Missing required fields in migration request")
        if not isinstance(instance_number, int) or instance_number < 0:
            abort(400, "Instance number must be a non-negative integer")

        target_node_id = migrationRequest["target_node_id"]
        if not target_node_id:
            abort(400, "Target node ID cannot be empty")
        target_node = mongo_find_node_by_id(target_node_id)
        if not target_node:
            # If target not not found by ID, try by name
            target_node = mongo_find_node_by_name(target_node_id)
        if (not target_node) or (not target_node.get("_id")):
            # If still not found, abort with 404
            abort(404, "Target node not found") 

        migrating_job = None
        migration_lock.acquire()
        try:
            # Set migration status to MIGRATING
            job = mongo_find_job_by_system_id_and_instance(system_job_id, instance_number)
            if not job:
                abort(404, "Job not found")
            if len(job["instance_list"])!=1:
                abort(404, "Job not found")
            status = job["instance_list"][0]["status"]
            if status not in [DeploymentStatus.RUNNING.value]:
                abort(400, "Job is not in a migratable state")
            if "target_node_id" not in migrationRequest:
                abort(400, "Target node ID is required for migration")
            target_node["_id"] = target_node.get("_id").__str__()  # Ensure _id is a string
            migrating_job = mongo_update_job_status(
                system_job_id,
                instance_number,
                DeploymentStatus.MIGRATION_REQUESTED,
                migration_target=target_node,
            )
            if not migrating_job:
                logging.error(
                    f"Failed to update job status for {system_job_id} instance {instance_number}"
                )
                abort(500, "Failed to update job status to MIGRATING")
        except Exception as e:
            logging.error(f"Error during migration request validation: {e}")
            migration_lock.release()
            abort(500, "Internal server error during migration request validation")
        finally:
            migration_lock.release()

        # init migration
        service_operations.service_migration(
            migrating_job, instance_number, target_node
        )

        return Response(
            json_util.dumps({"status": "migration request sent to edge nodes"}), 
            mimetype='application/json'
            )

"""Service management blueprints for the cluster manager.

Provides endpoints for deploying, deleting, and listing services
with pagination support. Integrates with the scheduling result handler
to update job statuses and notify the network plugin.
"""

import logging
from typing import Any, Dict, List, Optional

from flask import Blueprint, jsonify, request

logger = logging.getLogger(__name__)

service_bp = Blueprint("services", __name__, url_prefix="/api/v1/services")


def _get_mongo_client():
    """Lazy-import and return the MongoDB client from app context."""
    from flask import current_app
    return current_app.config["MONGO_CLIENT"]


def _notify_network_plugin(service_id: str, action: str, node_id: Optional[str] = None):
    """Notify the network plugin about service lifecycle events.

    In a production deployment this would call the network plugin's
    gRPC/HTTP endpoint. For now it logs the event.
    """
    logger.info(
        "Network plugin notification: service=%s action=%s node=%s",
        service_id, action, node_id,
    )


def _handle_scheduling_result(job_id: str, status: str, node_id: Optional[str] = None):
    """Update job status in MongoDB and trigger downstream actions.

    Maps Go-side JobStatus values to MongoDB updates.
    """
    mongo = _get_mongo_client()
    update_doc = mongo.set("status", status)
    if node_id:
        update_doc["$set"]["node_id"] = node_id
    mongo.update_job(job_id, update_doc)
    logger.info("Scheduling result handled: job=%s status=%s node=%s", job_id, status, node_id)


# ------------------------------------------------------------------
# Endpoints
# ------------------------------------------------------------------

@service_bp.route("", methods=["POST"])
def deploy_service():
    """Deploy a new service.

    Expected JSON body:
    {
        "name": "my-service",
        "image": "my-image:latest",
        "replicas": 1,
        "resources": {"cpu": "100m", "memory": "128Mi"},
        "metadata": {"labels": {"app": "my-service"}}
    }
    """
    data = request.get_json()
    if not data:
        return jsonify({"error": "Request body is required"}), 400

    name = data.get("name")
    if not name:
        return jsonify({"error": "Service name is required"}), 400

    mongo = _get_mongo_client()

    # Check for existing service with the same name
    existing = mongo.find_jobs({"metadata.name": name})
    if existing:
        return jsonify({"error": f"Service '{name}' already exists"}), 409

    # Create job document (mirrors Go model.Job structure)
    job = {
        "_id": data.get("id"),
        "name": name,
        "image": data.get("image", "unknown"),
        "status": "JobStatusPending",
        "replicas": data.get("replicas", 1),
        "resources": data.get("resources", {}),
        "metadata": data.get("metadata", {}),
        "created_at": mongo.now_iso(),
        "updated_at": mongo.now_iso(),
    }

    job_id = mongo.insert_job(job)
    if not job_id:
        return jsonify({"error": "Failed to create service"}), 500

    # Trigger scheduling (in production this would call the scheduler)
    _handle_scheduling_result(job_id, "JobStatusRunning", node_id=data.get("node_id"))
    _notify_network_plugin(job_id, "deploy", data.get("node_id"))

    return jsonify({"id": job_id, "name": name, "status": "JobStatusPending"}), 201


@service_bp.route("", methods=["GET"])
def list_services():
    """List all services with optional pagination.

    Query params:
        page (int): page number (default: 1)
        page_size (int): items per page (default: 20, max: 100)
    """
    try:
        page = int(request.args.get("page", 1))
        page_size = int(request.args.get("page_size", 20))
    except (ValueError, TypeError):
        return jsonify({"error": "Invalid pagination parameters"}), 400

    page_size = min(max(page_size, 1), 100)
    page = max(page, 1)

    mongo = _get_mongo_client()
    all_jobs = mongo.find_jobs()

    total = len(all_jobs)
    start = (page - 1) * page_size
    end = start + page_size
    services = all_jobs[start:end]

    return jsonify({
        "services": services,
        "pagination": {
            "page": page,
            "page_size": page_size,
            "total": total,
            "total_pages": (total + page_size - 1) // page_size if page_size else 0,
        },
    }), 200


@service_bp.route("/<service_id>", methods=["GET"])
def get_service(service_id: str):
    """Retrieve a single service by ID."""
    mongo = _get_mongo_client()
    service = mongo.find_job(service_id)
    if not service:
        return jsonify({"error": "Service not found"}), 404
    return jsonify(service), 200


@service_bp.route("/<service_id>", methods=["DELETE"])
def delete_service(service_id: str):
    """Delete a service by ID."""
    mongo = _get_mongo_client()
    service = mongo.find_job(service_id)
    if not service:
        return jsonify({"error": "Service not found"}), 404

    # Update status before removing
    mongo.update_job(service_id, mongo.set("status", "JobStatusTerminated"))
    _notify_network_plugin(service_id, "delete", service.get("node_id"))

    # In production, also remove from MongoDB
    mongo._db["jobs"].delete_one({"_id": service_id})
    logger.info("Deleted service %s", service_id)

    return jsonify({"id": service_id, "deleted": True}), 200

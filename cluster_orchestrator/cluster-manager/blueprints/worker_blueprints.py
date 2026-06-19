"""Worker node blueprints for the cluster manager.

Provides endpoints for worker node registration and retrieval.
Registration includes token handling and MQTT broker port configuration.
"""

import logging
import secrets
from typing import Any, Dict, Optional

from flask import Blueprint, jsonify, request

logger = logging.getLogger(__name__)

worker_bp = Blueprint("workers", __name__, url_prefix="/api/v1/workers")


def _get_mongo_client():
    """Lazy-import and return the MongoDB client from app context."""
    from flask import current_app
    return current_app.config["MONGO_CLIENT"]


# ------------------------------------------------------------------
# Endpoints
# ------------------------------------------------------------------

@worker_bp.route("", methods=["POST"])
def register_node():
    """Register a new worker node.

    Expected JSON body:
    {
        "node_id": "node-123",
        "token": "registration-token",
        "hostname": "worker-1.example.com",
        "mqtt_broker_port": 1883,
        "resources": {"cpu": "4", "memory": "8Gi"},
        "labels": {"zone": "us-east-1"}
    }

    Returns the node document with a generated registration token.
    """
    data = request.get_json()
    if not data:
        return jsonify({"error": "Request body is required"}), 400

    node_id = data.get("node_id")
    token = data.get("token")

    if not node_id:
        return jsonify({"error": "node_id is required"}), 400
    if not token:
        return jsonify({"error": "Registration token is required"}), 400

    mongo = _get_mongo_client()

    # Check if node already exists
    existing = mongo.find_node(node_id)
    if existing:
        return jsonify({"error": f"Node {node_id} already registered"}), 409

    # Generate a long-lived auth token for the node
    auth_token = secrets.token_hex(32)

    # Create node document (mirrors Go model.Node structure)
    node = {
        "_id": node_id,
        "hostname": data.get("hostname", ""),
        "status": "NodeStatusReady",
        "migration_status": "NodeMigrationStatusNone",
        "resources": data.get("resources", {}),
        "labels": data.get("labels", {}),
        "mqtt_broker_port": data.get("mqtt_broker_port", 1883),
        "auth_token": auth_token,
        "registered_at": mongo.now_iso(),
        "updated_at": mongo.now_iso(),
    }

    inserted_id = mongo.insert_node(node)
    if not inserted_id:
        return jsonify({"error": "Failed to register node"}), 500

    logger.info("Node registered: node=%s mqtt_port=%s", node_id, node["mqtt_broker_port"])

    # Return node without the auth token
    safe_node = {k: v for k, v in node.items() if k != "auth_token"}
    return jsonify({
        "node": safe_node,
        "auth_token": auth_token,
    }), 201


@worker_bp.route("", methods=["GET"])
def list_workers():
    """List all registered worker nodes."""
    mongo = _get_mongo_client()
    nodes = mongo.find_nodes()

    # Strip auth tokens from response
    safe_nodes = [
        {k: v for k, v in node.items() if k != "auth_token"}
        for node in nodes
    ]

    return jsonify({"workers": safe_nodes}), 200


@worker_bp.route("/<node_id>", methods=["GET"])
def get_worker(node_id: str):
    """Retrieve a single worker node by ID."""
    mongo = _get_mongo_client()
    node = mongo.find_node(node_id)
    if not node:
        return jsonify({"error": "Worker node not found"}), 404

    # Strip auth token from response
    safe_node = {k: v for k, v in node.items() if k != "auth_token"}
    return jsonify(safe_node), 200


@worker_bp.route("/<node_id>", methods=["DELETE"])
def deregister_node(node_id: str):
    """Deregister a worker node."""
    mongo = _get_mongo_client()
    node = mongo.find_node(node_id)
    if not node:
        return jsonify({"error": "Worker node not found"}), 404

    mongo.delete_node(node_id)
    logger.info("Node deregistered: node=%s", node_id)

    return jsonify({"id": node_id, "deleted": True}), 200

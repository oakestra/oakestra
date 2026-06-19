"""Migration blueprints for the cluster manager.

Provides endpoints for job migration operations including validation,
status updates, and initiating migration requests. Uses thread-safe
migration locking to ensure consistency during concurrent migrations.
"""

import logging
import threading
from typing import Any, Dict, Optional

from flask import Blueprint, jsonify, request

logger = logging.getLogger(__name__)

migration_bp = Blueprint("migrations", __name__, url_prefix="/api/v1/migrations")

# Thread-safe migration locking: maps migration_id -> Lock
_migration_locks: Dict[str, threading.Lock] = {}
_migration_locks_mutex = threading.Lock()


def _get_migration_lock(migration_id: str) -> threading.Lock:
    """Get or create a lock for the given migration_id.

    Thread-safe: uses a mutex to protect the _migration_locks dict.
    """
    with _migration_locks_mutex:
        if migration_id not in _migration_locks:
            _migration_locks[migration_id] = threading.Lock()
        return _migration_locks[migration_id]


def _get_mongo_client():
    """Lazy-import and return the MongoDB client from app context."""
    from flask import current_app
    return current_app.config["MONGO_CLIENT"]


def _validate_migration_request(data: Dict[str, Any]) -> Optional[str]:
    """Validate migration request fields.

    Returns an error message string if validation fails, or None if valid.
    """
    if not data.get("job_id"):
        return "job_id is required"
    if not data.get("target_node_id"):
        return "target_node_id is required"
    return None


# ------------------------------------------------------------------
# Endpoints
# ------------------------------------------------------------------

@migration_bp.route("", methods=["POST"])
def create_migration():
    """Initiate a migration request for a job.

    Expected JSON body:
    {
        "job_id": "job-123",
        "target_node_id": "node-456",
        "reason": "node maintenance"
    }

    Returns the migration ID on success.
    """
    data = request.get_json()
    if not data:
        return jsonify({"error": "Request body is required"}), 400

    # Validate request
    error = _validate_migration_request(data)
    if error:
        return jsonify({"error": error}), 400

    job_id = data["job_id"]
    target_node_id = data["target_node_id"]
    reason = data.get("reason", "manual")

    mongo = _get_mongo_client()

    # Check that the job exists
    job = mongo.find_job(job_id)
    if not job:
        return jsonify({"error": f"Job {job_id} not found"}), 404

    # Check that the target node exists
    target_node = mongo.find_node(target_node_id)
    if not target_node:
        return jsonify({"error": f"Target node {target_node_id} not found"}), 404

    # Check that the job is not already migrating
    if job.get("status") == "JobStatusMigrating":
        return jsonify({"error": f"Job {job_id} is already migrating"}), 409

    # Check that the target node is available
    if target_node.get("status") != "NodeStatusReady":
        return jsonify({"error": f"Target node {target_node_id} is not ready (status: {target_node.get('status')})"}), 409

    # Create migration document
    migration_id = f"migration-{job_id}-{target_node_id}"
    migration = {
        "_id": migration_id,
        "job_id": job_id,
        "source_node_id": job.get("node_id"),
        "target_node_id": target_node_id,
        "status": "MigrationStatusPending",
        "reason": reason,
        "created_at": mongo.now_iso(),
        "updated_at": mongo.now_iso(),
    }

    inserted_id = mongo.insert_migration(migration)
    if not inserted_id:
        return jsonify({"error": "Failed to create migration"}), 500

    # Update job status to migrating
    mongo.update_job(job_id, {
        "$set": {
            "status": "JobStatusMigrating",
            "migration_id": migration_id,
            "updated_at": mongo.now_iso(),
        }
    })

    # Update source node status
    source_node_id = job.get("node_id")
    if source_node_id:
        mongo.update_node(source_node_id, {
            "$set": {
                "migration_status": "NodeMigrationStatusMigrating",
                "updated_at": mongo.now_iso(),
            }
        })

    # Acquire lock and initiate migration
    lock = _get_migration_lock(migration_id)
    with lock:
        migration["status"] = "MigrationStatusInProgress"
        mongo.update_migration(migration_id, {
            "$set": {
                "status": "MigrationStatusInProgress",
                "updated_at": mongo.now_iso(),
            }
        })
        logger.info("Migration initiated: migration=%s job=%s source=%s target=%s",
                     migration_id, job_id, source_node_id, target_node_id)

    return jsonify({
        "id": migration_id,
        "job_id": job_id,
        "target_node_id": target_node_id,
        "status": "MigrationStatusInProgress",
    }), 201


@migration_bp.route("/<migration_id>", methods=["GET"])
def get_migration(migration_id: str):
    """Retrieve migration status by ID."""
    mongo = _get_mongo_client()
    migration = mongo.find_migration(migration_id)
    if not migration:
        return jsonify({"error": "Migration not found"}), 404
    return jsonify(migration), 200


@migration_bp.route("/<migration_id>/cancel", methods=["POST"])
def cancel_migration(migration_id: str):
    """Cancel an in-progress migration."""
    mongo = _get_mongo_client()
    migration = mongo.find_migration(migration_id)
    if not migration:
        return jsonify({"error": "Migration not found"}), 404

    if migration.get("status") not in ("MigrationStatusPending", "MigrationStatusInProgress"):
        return jsonify({"error": f"Cannot cancel migration in status {migration.get('status')}"}), 409

    lock = _get_migration_lock(migration_id)
    with lock:
        mongo.update_migration(migration_id, {
            "$set": {
                "status": "MigrationStatusCancelled",
                "updated_at": mongo.now_iso(),
            }
        })

        # Restore job status
        job_id = migration.get("job_id")
        if job_id:
            mongo.update_job(job_id, {
                "$set": {
                    "status": "JobStatusRunning",
                    "updated_at": mongo.now_iso(),
                }
            })

        logger.info("Migration cancelled: migration=%s", migration_id)

    return jsonify({"id": migration_id, "status": "MigrationStatusCancelled"}), 200


@migration_bp.route("", methods=["GET"])
def list_migrations():
    """List all migrations with optional job_id filter."""
    job_id = request.args.get("job_id")

    mongo = _get_mongo_client()
    if job_id:
        migrations = mongo.find_jobs({"migration_id": job_id})
        # Filter to only migration documents
        migrations = [m for m in mongo._db["migrations"].find({"job_id": job_id})]
    else:
        migrations = list(mongo._db["migrations"].find())

    return jsonify({"migrations": migrations}), 200

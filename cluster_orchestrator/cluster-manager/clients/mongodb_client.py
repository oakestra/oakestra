"""MongoDB client for cluster manager.

Provides typed query methods for nodes, jobs, and migrations.
"""

import logging
from datetime import datetime
from typing import Any, Dict, List, Optional

from pymongo import MongoClient, errors
from pymongo.database import Database

logger = logging.getLogger(__name__)

# Collection names
NODES_COLLECTION = "nodes"
JOBS_COLLECTION = "jobs"
MIGRATIONS_COLLECTION = "migrations"


class MongoDBClient:
    """Thin wrapper around PyMongo with cluster-manager-specific helpers."""

    def __init__(self, uri: str, db_name: str = "oakestra"):
        self._client = MongoClient(uri, serverSelectionTimeoutMS=5000)
        self._db: Database = self._client[db_name]
        logger.info("Connected to MongoDB at %s (db=%s)", uri, db_name)

    # ------------------------------------------------------------------
    # Nodes
    # ------------------------------------------------------------------

    def find_node(self, node_id: str) -> Optional[Dict[str, Any]]:
        """Return a single node document by _id, or None."""
        try:
            return self._db[NODES_COLLECTION].find_one({"_id": node_id})
        except errors.OperationFailure as exc:
            logger.error("Failed to find node %s: %s", node_id, exc)
            return None

    def find_nodes(self, filter_doc: Optional[Dict[str, Any]] = None) -> List[Dict[str, Any]]:
        """Return all nodes matching *filter_doc* (default: all nodes)."""
        try:
            cursor = self._db[NODES_COLLECTION].find(filter_doc or {})
            return list(cursor)
        except errors.OperationFailure as exc:
            logger.error("Failed to query nodes with filter %s: %s", filter_doc, exc)
            return []

    def find_nodes_by_status(self, status: str) -> List[Dict[str, Any]]:
        """Convenience: find nodes by their status field."""
        return self.find_nodes({"status": status})

    def insert_node(self, node: Dict[str, Any]) -> Optional[str]:
        """Insert a node document. Returns the inserted _id on success."""
        try:
            result = self._db[NODES_COLLECTION].insert_one(node)
            logger.info("Inserted node %s", result.inserted_id)
            return str(result.inserted_id)
        except errors.DuplicateKeyError:
            logger.warning("Node %s already exists, skipping insert", node.get("_id"))
            return None
        except errors.OperationFailure as exc:
            logger.error("Failed to insert node: %s", exc)
            return None

    def update_node(self, node_id: str, update_doc: Dict[str, Any]) -> bool:
        """Update a node document using $-prefixed operators (e.g. {"$set": ...}).

        Uses an explicit filter instead of upsert to avoid silent document creation.
        """
        try:
            result = self._db[NODES_COLLECTION].update_one(
                {"_id": node_id},
                update_doc,
            )
            if result.matched_count == 0:
                logger.warning("Node %s not found, nothing to update", node_id)
                return False
            logger.info("Updated node %s (%d modified)", node_id, result.modified_count)
            return True
        except errors.OperationFailure as exc:
            logger.error("Failed to update node %s: %s", node_id, exc)
            return False

    def delete_node(self, node_id: str) -> bool:
        """Delete a node document. Returns True if a document was removed."""
        try:
            result = self._db[NODES_COLLECTION].delete_one({"_id": node_id})
            deleted = result.deleted_count > 0
            if deleted:
                logger.info("Deleted node %s", node_id)
            else:
                logger.warning("Node %s not found for deletion", node_id)
            return deleted
        except errors.OperationFailure as exc:
            logger.error("Failed to delete node %s: %s", node_id, exc)
            return False

    # ------------------------------------------------------------------
    # Jobs
    # ------------------------------------------------------------------

    def find_job(self, job_id: str) -> Optional[Dict[str, Any]]:
        """Return a single job document by _id."""
        try:
            return self._db[JOBS_COLLECTION].find_one({"_id": job_id})
        except errors.OperationFailure as exc:
            logger.error("Failed to find job %s: %s", job_id, exc)
            return None

    def find_jobs(self, filter_doc: Optional[Dict[str, Any]] = None) -> List[Dict[str, Any]]:
        """Return all jobs matching *filter_doc*."""
        try:
            cursor = self._db[JOBS_COLLECTION].find(filter_doc or {})
            return list(cursor)
        except errors.OperationFailure as exc:
            logger.error("Failed to query jobs with filter %s: %s", filter_doc, exc)
            return []

    def update_job(self, job_id: str, update_doc: Dict[str, Any]) -> bool:
        """Update a job document using $-prefixed operators."""
        try:
            result = self._db[JOBS_COLLECTION].update_one(
                {"_id": job_id},
                update_doc,
            )
            if result.matched_count == 0:
                logger.warning("Job %s not found, nothing to update", job_id)
                return False
            logger.info("Updated job %s (%d modified)", job_id, result.modified_count)
            return True
        except errors.OperationFailure as exc:
            logger.error("Failed to update job %s: %s", job_id, exc)
            return False

    # ------------------------------------------------------------------
    # Migrations
    # ------------------------------------------------------------------

    def find_migration(self, migration_id: str) -> Optional[Dict[str, Any]]:
        """Return a migration document by _id."""
        try:
            return self._db[MIGRATIONS_COLLECTION].find_one({"_id": migration_id})
        except errors.OperationFailure as exc:
            logger.error("Failed to find migration %s: %s", migration_id, exc)
            return None

    def insert_migration(self, migration: Dict[str, Any]) -> Optional[str]:
        """Insert a migration document. Returns the inserted _id."""
        try:
            result = self._db[MIGRATIONS_COLLECTION].insert_one(migration)
            logger.info("Inserted migration %s", result.inserted_id)
            return str(result.inserted_id)
        except errors.DuplicateKeyError:
            logger.warning("Migration %s already exists", migration.get("_id"))
            return None
        except errors.OperationFailure as exc:
            logger.error("Failed to insert migration: %s", exc)
            return None

    def update_migration(self, migration_id: str, update_doc: Dict[str, Any]) -> bool:
        """Update a migration document using $-prefixed operators."""
        try:
            result = self._db[MIGRATIONS_COLLECTION].update_one(
                {"_id": migration_id},
                update_doc,
            )
            if result.matched_count == 0:
                logger.warning("Migration %s not found, nothing to update", migration_id)
                return False
            logger.info("Updated migration %s (%d modified)", migration_id, result.modified_count)
            return True
        except errors.OperationFailure as exc:
            logger.error("Failed to update migration %s: %s", migration_id, exc)
            return False

    # ------------------------------------------------------------------
    # Helpers
    # ------------------------------------------------------------------

    def now_iso(self) -> str:
        """Return the current UTC time as an ISO-8601 string."""
        return datetime.utcnow().isoformat()

    def set(self, field: str, value: Any) -> Dict[str, Any]:
        """Convenience: build a {"$set": {field: value}} document."""
        return {"$set": {field: value}}

    @property
    def db(self) -> Database:
        """Expose the raw Database for advanced queries."""
        return self._db

    def close(self) -> None:
        """Close the MongoDB connection."""
        self._client.close()
        logger.info("MongoDB connection closed")

import logging
from datetime import datetime

from ext_requests import mongodb_client

logger = logging.getLogger("system_manager")


def create_cluster_token(cluster_name, token):
    """
    Save or replace a cluster token in the database.
    
    Args:
        cluster_name: The name of the cluster
        token: The JWT token string
    
    Returns:
        The stored document
    """
    collection = mongodb_client.mongo_cluster_tokens
    if collection is None:
        logger.error("Cluster tokens database not initialized")
        return None
    
    token_doc = {
        "cluster_name": cluster_name,
        "token": token,
        "used": False,
        "created_at": datetime.utcnow(),
        "updated_at": datetime.utcnow(),
    }
    
    try:
        collection.update_one(
            {"cluster_name": cluster_name},
            {
                "$set": {
                    "token": token,
                    "used": False,
                    "updated_at": token_doc["updated_at"],
                },
                "$setOnInsert": {"created_at": token_doc["created_at"]},
                "$unset": {"used_at": ""},
            },
            upsert=True,
        )
        logger.info(f"Cluster token saved for: {cluster_name}")
        return collection.find_one({"cluster_name": cluster_name})
    except Exception as e:
        logger.error(f"Failed to save cluster token for {cluster_name}: {str(e)}")
        return None


def get_cluster_token(cluster_name):
    """
    Retrieve the token for a cluster.
    
    Args:
        cluster_name: The name of the cluster
    
    Returns:
        The token document, or None if not found
    """
    collection = mongodb_client.mongo_cluster_tokens
    if collection is None:
        logger.error("Cluster tokens database not initialized")
        return None
    
    try:
        return collection.find_one({"cluster_name": cluster_name}, sort=[("created_at", -1)])
    except Exception as e:
        logger.error(f"Failed to retrieve cluster token for {cluster_name}: {str(e)}")
        return None


def mark_cluster_token_used(cluster_name, token):
    """
    Mark a cluster token as used.

    Args:
        cluster_name: The cluster name tied to the token
        token: The token to mark as used

    Returns:
        True if a token was marked as used, False otherwise
    """
    collection = mongodb_client.mongo_cluster_tokens
    if collection is None:
        logger.error("Cluster tokens database not initialized")
        return False

    try:
        result = collection.update_one(
            {
                "cluster_name": cluster_name,
                "token": token,
                "$or": [{"used": {"$exists": False}}, {"used": False}],
            },
            {"$set": {"used": True, "used_at": datetime.utcnow()}},
        )
        return result.modified_count == 1
    except Exception as e:
        logger.error(f"Failed to mark token as used for {cluster_name}: {str(e)}")
        return False


def verify_cluster_token(cluster_name, provided_token):
    """
    Verify that the provided token matches the stored token for a cluster.
    
    Args:
        cluster_name: The name of the cluster
        provided_token: The token provided by the cluster
    
    Returns:
        True if token is valid and matches, False otherwise
    """
    token_doc = get_cluster_token(cluster_name)
    
    if not token_doc:
        logger.warning(f"No token found for cluster: {cluster_name}")
        return False
    
    stored_token = token_doc.get("token")
    if provided_token == stored_token:
        logger.info(f"Cluster token verified for: {cluster_name}")
        return True
    else:
        logger.warning(f"Invalid token provided for cluster: {cluster_name}")
        return False


def delete_cluster_token(cluster_name):
    """
    Delete a cluster token (e.g., after successful registration or token rotation).
    
    Args:
        cluster_name: The name of the cluster
    
    Returns:
        Number of deleted documents
    """
    collection = mongodb_client.mongo_cluster_tokens
    if collection is None:
        logger.error("Cluster tokens database not initialized")
        return 0
    
    try:
        result = collection.delete_one({"cluster_name": cluster_name})
        logger.info(f"Cluster token deleted for: {cluster_name}")
        return result.deleted_count
    except Exception as e:
        logger.error(f"Failed to delete cluster token for {cluster_name}: {str(e)}")
        return 0

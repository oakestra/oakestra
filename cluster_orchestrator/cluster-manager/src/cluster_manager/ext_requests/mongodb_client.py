import os

from pymongo import MongoClient

CLUSTER_MONGO_URL = os.environ.get("CLUSTER_MONGO_URL", "localhost")
CLUSTER_MONGO_PORT = os.environ.get("CLUSTER_MONGO_PORT", 10107)

_client = None


def get_collection(name: str):
    """Return a collection of the "clusters" database, connecting on first use."""
    global _client
    if _client is None:
        _client = MongoClient(f"mongodb://{CLUSTER_MONGO_URL}:{CLUSTER_MONGO_PORT}/")
    return _client["clusters"][name]

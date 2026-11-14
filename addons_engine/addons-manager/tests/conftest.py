"""Test setup for the addons-manager.

addons_manager calls mongo_init on import, which would connect to Mongo. We stub flask_pymongo
before the app is imported, and each test gets an in-memory FakeCollection as mongo_addons.
"""

import sys
from types import SimpleNamespace
from unittest.mock import MagicMock

import pytest
from bson.objectid import ObjectId

sys.modules["flask_pymongo"] = MagicMock()

import db.mongodb_client as mongodb_client  # noqa: E402
from addons_manager import app  # noqa: E402


def _matches(doc, query):
    for key, condition in query.items():
        if isinstance(condition, dict) and "$ne" in condition:
            if doc.get(key) == condition["$ne"]:
                return False
        elif doc.get(key) != condition:
            return False
    return True


class FakeCollection:
    """Minimal stand-in for a pymongo collection. Only supports equality and $ne filters."""

    def __init__(self, docs=()):
        self.docs = [dict(doc) for doc in docs]
        self.queries = []

    def find(self, query=None):
        query = query or {}
        self.queries.append(query)
        return [dict(doc) for doc in self.docs if _matches(doc, query)]

    def find_one(self, query):
        found = self.find(query)
        return found[0] if found else None

    def insert_one(self, doc):
        doc = {**doc, "_id": ObjectId()}
        self.docs.append(doc)
        return SimpleNamespace(inserted_id=doc["_id"])

    def find_one_and_update(self, query, update, return_document=False):
        for doc in self.docs:
            if _matches(doc, query):
                doc.update(update["$set"])
                return dict(doc)
        return None

    def find_one_and_delete(self, query):
        for doc in self.docs:
            if _matches(doc, query):
                self.docs.remove(doc)
                return doc
        return None


@pytest.fixture
def addons_collection(monkeypatch):
    collection = FakeCollection()
    monkeypatch.setattr(mongodb_client, "mongo_addons", collection)
    return collection


@pytest.fixture
def client(addons_collection):
    app.config["TESTING"] = True
    return app.test_client()

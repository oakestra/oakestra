from bson import ObjectId
from flask.json.provider import DefaultJSONProvider


class MongoJSONProvider(DefaultJSONProvider):
    @staticmethod
    def default(obj):
        if isinstance(obj, ObjectId):
            return str(obj)
        return DefaultJSONProvider.default(obj)

from bson import ObjectId
from flask.json import JSONEncoder


class MongoJSONEncoder(JSONEncoder):
    def default(self, obj):
        if isinstance(obj, ObjectId):
            return str(obj)
        return super().default(obj)

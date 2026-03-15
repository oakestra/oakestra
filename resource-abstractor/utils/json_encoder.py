from flask.json import JSONEncoder
from bson import ObjectId


# To handle ObjectId serialization in JSON responses
class MongoJSONEncoder(JSONEncoder):
    def default(self, obj):
        if isinstance(obj, ObjectId):
            return str(obj)
        return super().default(obj)

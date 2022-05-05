import traceback

from bson import json_util
from flask import request, Response, jsonify
from flask_restful import Resource


# ........ Functions for user management ...............#
# ......................................................#
class SchedulingController(Resource):

    @staticmethod
    def post(appid):
        try:
            return Response(
                status=200,
            )
        except Exception as e:
            tb = traceback.format_exc()
            print(tb)
            return {"message": e.message}, 404


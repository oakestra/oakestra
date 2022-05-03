from flask_restful import Resource
from roles.securityUtils import jwt_auth_required
from services.instance_management import scale_down_instance, scale_up_instance


class DeployInstanceController(Resource):
    @staticmethod
    @jwt_auth_required()
    def post(serviceid):
        username = jwt_auth_required()
        scale_up_instance(serviceid, username)
        return {}, 200

    @staticmethod
    @jwt_auth_required()
    def delete(serviceid, instance_number):
        username = jwt_auth_required()
        scale_down_instance(serviceid, username, how_many=instance_number)
        return 200

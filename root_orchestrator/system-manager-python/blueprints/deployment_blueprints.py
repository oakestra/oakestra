from flask_restful import Resource
from roles.securityUtils import jwt_auth_required
from services.instance_management import scale_down_instance, scale_up_instance
from flask_smorest import Blueprint, Api, abort

deploybp = Blueprint(
    'Deployment', 'deployment', url_prefix='/api/service'
)


@deploybp.route('/<serviceid>/instance')
class DeployInstanceController(Resource):
    @deploybp.response(200, content_type="application/json")
    @jwt_auth_required()
    def post(self, serviceid):
        username = jwt_auth_required()
        scale_up_instance(serviceid, username)
        return {}


@deploybp.route('/<serviceid>/instance/<instance_number>')
class UndeployInstanceController(Resource):
    @deploybp.response(200, content_type="application/json")
    @jwt_auth_required()
    def delete(self, serviceid, instance_number):
        username = jwt_auth_required()
        scale_down_instance(serviceid, username, how_many=instance_number)
        return 200

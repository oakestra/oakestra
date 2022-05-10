from flask.views import MethodView
from roles.securityUtils import jwt_auth_required
from services.instance_management import request_scale_down_instance, request_scale_up_instance
from flask_smorest import Blueprint, Api, abort

deploybp = Blueprint(
    'Deployment', 'deployment', url_prefix='/api/service'
)


@deploybp.route('/<serviceid>/instance')
class DeployInstanceController(MethodView):
    @deploybp.response(200, content_type="application/json")
    @jwt_auth_required()
    def post(self, serviceid):
        username = jwt_auth_required()
        request_scale_up_instance(serviceid, username)
        return {}


@deploybp.route('/<serviceid>/instance/<instance_number>')
class UndeployInstanceController(MethodView):
    @deploybp.response(200, content_type="application/json")
    @jwt_auth_required()
    def delete(self, serviceid, instance_number):
        username = jwt_auth_required()
        request_scale_down_instance(serviceid, username, how_many=instance_number)
        return 200

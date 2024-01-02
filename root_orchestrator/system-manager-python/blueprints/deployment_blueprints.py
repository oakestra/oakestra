from flask.views import MethodView
from flask_jwt_extended import get_jwt_identity
from flask_smorest import Blueprint
from roles.securityUtils import jwt_auth_required
from services.instance_management import request_scale_down_instance, request_scale_up_instance

deploybp = Blueprint("Deployment", "deployment", url_prefix="/api/service")


@deploybp.route("/<serviceid>/instance")
class DeployInstanceController(MethodView):
    @jwt_auth_required()
    def post(self, serviceid):
        username = get_jwt_identity()
        request_scale_up_instance(str(serviceid), username)
        return {"message": "ok"}


@deploybp.route("/<serviceid>/instance/<instance_number>")
class UndeployInstanceController(MethodView):
    @jwt_auth_required()
    def delete(self, serviceid, instance_number):
        username = get_jwt_identity()
        request_scale_down_instance(serviceid, username, which_one=int(instance_number))
        return {"message": "ok"}

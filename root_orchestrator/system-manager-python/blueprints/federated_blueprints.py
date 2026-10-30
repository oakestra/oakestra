import logging

from flask import request
from flask.views import MethodView
from flask_smorest import abort
from users.federated import FederatedLoginError, federated_login, login_methods

from blueprints.jwt_wrapper import BlueprintExt

logger = logging.getLogger("system_manager")

federatedbp = BlueprintExt("Federated Login", "federated_auth", url_prefix="/api/auth")

federated_schema = {
    "type": "object",
    "properties": {
        "token": {"type": "string"},
    },
}


# ......... Functions for the federated login ..........#
# ......................................................#


@federatedbp.route("/methods")
class LoginMethodsController(MethodView):
    def get(self, *args, **kwargs):
        """Which login methods this cluster accepts, so clients can show the right sign-in."""
        return login_methods()


@federatedbp.route("/federated")
class FederatedLoginController(MethodView):
    @federatedbp.arguments(schema=federated_schema, location="json", validate=False, unknown=True)
    def post(self, *args, **kwargs):
        content = request.get_json(silent=True) or {}
        try:
            return federated_login(content.get("token"))
        except FederatedLoginError as e:
            abort(e.status, message=e.message)

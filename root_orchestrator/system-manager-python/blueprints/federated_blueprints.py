import logging

from flask import Response, redirect, request
from flask.views import MethodView
from flask_smorest import abort
from roles.securityUtils import Role, jwt_auth_required, require_role
from users import oidc, sessions
from users.federated import login_methods
from users.user_management import user_get_by_name

from blueprints.jwt_wrapper import BlueprintExt

logger = logging.getLogger("system_manager")

federatedbp = BlueprintExt("Federated Login", "federated_auth", url_prefix="/api/auth")
signoutbp = BlueprintExt("User Sign-out", "user_signout", url_prefix="/api/users")

code_schema = {"type": "object", "properties": {"code": {"type": "string"}}}
device_schema = {"type": "object", "properties": {"device_code": {"type": "string"}}}


# ......... Functions for the OIDC login ...............#
# ......................................................#


@federatedbp.route("/methods")
class LoginMethodsController(MethodView):
    def get(self, *args, **kwargs):
        """Which login methods this cluster accepts, so clients can show the right sign-in."""
        return login_methods()


@federatedbp.route("/oidc/login")
class OidcLoginController(MethodView):
    def get(self, *args, **kwargs):
        """Redirects the browser to the identity provider; it comes back to /oidc/callback."""
        try:
            return redirect(oidc.login_url(request.args.get("return_to"), request.host))
        except oidc.OidcError as e:
            abort(e.status, message=e.message)


@federatedbp.route("/oidc/callback")
class OidcCallbackController(MethodView):
    def get(self, *args, **kwargs):
        """Provider redirect target: back to return_to#oidc_code=... (or #oidc_error=...)."""
        return_to, key, value = oidc.callback(request.args)
        if return_to is None:
            return Response(f"Sign-in failed: {value}\n", status=400, mimetype="text/plain")
        return redirect(oidc.with_fragment(return_to, key, value))


@federatedbp.route("/oidc/exchange")
class OidcExchangeController(MethodView):
    @federatedbp.arguments(schema=code_schema, location="json", validate=False, unknown=True)
    def post(self, *args, **kwargs):
        """Trades a one-time code from the callback (or users.emergency) for Oakestra tokens."""
        content = request.get_json(silent=True) or {}
        try:
            return oidc.exchange(content.get("code"))
        except oidc.OidcError as e:
            return {"message": e.message}, 400


@federatedbp.route("/oidc/device")
class OidcDeviceController(MethodView):
    def post(self, *args, **kwargs):
        """Starts a device login (CLI) at the identity provider for this cluster's client."""
        try:
            return oidc.device_start()
        except oidc.OidcError as e:
            return {"message": e.message}, e.status


@federatedbp.route("/oidc/device/token")
class OidcDeviceTokenController(MethodView):
    @federatedbp.arguments(schema=device_schema, location="json", validate=False, unknown=True)
    def post(self, *args, **kwargs):
        """Polls a device login; 400 with error=authorization_pending until the user approves."""
        content = request.get_json(silent=True) or {}
        try:
            return oidc.device_token(content.get("device_code"))
        except oidc.OidcError as e:
            if e.status == 400:
                return {"error": e.code, "message": e.message}, 400
            return {"message": e.message}, e.status


@federatedbp.route("/signout-all")
class SignoutAllController(MethodView):
    @jwt_auth_required()
    @require_role(Role.ADMIN)
    def post(self, *args, **kwargs):
        """Signs out every user, including the caller. Allowed while user management is locked."""
        sessions.signout_all()
        return {"message": "Everyone has been signed out"}


@signoutbp.route("/<username>/signout")
class UserSignoutController(MethodView):
    @jwt_auth_required()
    @require_role(Role.ADMIN)
    def post(self, username, *args, **kwargs):
        """Signs out one user. Allowed while user management is locked."""
        if user_get_by_name(username, None) is None:
            return {"message": "User does not exist"}, 404
        sessions.signout_user(username)
        return {"message": f"{username} has been signed out"}

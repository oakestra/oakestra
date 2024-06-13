from db import hooks_db
from flask.views import MethodView
from flask_smorest import Blueprint
from marshmallow import Schema, fields, validate

hooksblp = Blueprint("Hooks", "hooks", url_prefix="/api/v1/hooks")


class APIObjectHookSchema(Schema):
    _id = fields.String()
    hook_name = fields.String(required=True)
    webhook_url = fields.String(required=True)
    entity = fields.String(required=True)
    events = fields.List(
        fields.Str(validate=validate.OneOf([*hooks_db.ASYNC_EVENTS, *hooks_db.SYNC_EVENTS])),
        default=[],
    )


@hooksblp.route("/")
class AllHooksController(MethodView):
    @hooksblp.response(200, APIObjectHookSchema(many=True), content_type="application/json")
    def get(self, *args, **kwargs):
        return hooks_db.find_hooks()

    @hooksblp.arguments(APIObjectHookSchema, location="json")
    @hooksblp.response(200, APIObjectHookSchema, content_type="application/json")
    def put(self, data, *args, **kwargs):
        return hooks_db.create_update_hook(data)


@hooksblp.route("/<hookId>")
class SingleHookController(MethodView):
    @hooksblp.response(200, APIObjectHookSchema, content_type="application/json")
    def get(self, hookId, *args, **kwargs):
        hook = hooks_db.find_hook_by_id(hookId)
        if not hook:
            return "Hook not found", 404

        return hook

    @hooksblp.response(204, content_type="application/json")
    def delete(self, hookId, *args, **kwargs):
        hooks_db.delete_hook(hookId)

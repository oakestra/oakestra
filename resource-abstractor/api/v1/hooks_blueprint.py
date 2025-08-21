from db import hooks_db
from flask.views import MethodView
from flask_smorest import Blueprint
from marshmallow import Schema, fields, validate

hooksblp = Blueprint("Hooks", "hooks", url_prefix="/api/v1/hooks")


class APIObjectPostHookSchema(Schema):
    hook_name = fields.String()
    webhook_url = fields.String()
    entity = fields.String()
    events = fields.List(
        fields.Str(validate=validate.OneOf([*hooks_db.ASYNC_EVENTS, *hooks_db.SYNC_EVENTS])),
        default=[],
    )


class APIObjectHookSchema(APIObjectPostHookSchema):
    _id = fields.String()


@hooksblp.route("/")
class AllHooksController(MethodView):
    @hooksblp.response(200, APIObjectHookSchema(many=True), content_type="application/json")
    def get(self, *args, **kwargs):
        return hooks_db.find_hooks()

    @hooksblp.arguments(APIObjectPostHookSchema, location="json")
    @hooksblp.response(201, APIObjectHookSchema, content_type="application/json")
    def post(self, data, *args, **kwargs):
        return hooks_db.create_hook(data)


@hooksblp.route("/<hook_id>")
class SingleHookController(MethodView):
    @hooksblp.response(200, APIObjectHookSchema, content_type="application/json")
    def get(self, hook_id, *args, **kwargs):
        hook = hooks_db.find_hook_by_id(hook_id)
        if not hook:
            return "Hook not found", 404

        return hook

    @hooksblp.response(204, content_type="application/json")
    def delete(self, hook_id, *args, **kwargs):
        hooks_db.delete_hook(hook_id)

    @hooksblp.arguments(APIObjectPostHookSchema, validate=False, location="json")
    @hooksblp.response(200, APIObjectHookSchema, content_type="application/json")
    def patch(self, data, *args, **kwargs):
        hook_id = kwargs.get("hook_id")
        hook = hooks_db.update_hook(hook_id, data)

        return hook

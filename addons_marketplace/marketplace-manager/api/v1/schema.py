import re

from marshmallow import Schema, ValidationError, fields, validate, validates_schema

# The dashboard proxies /addon-assets/<name>/ and resolves the container addonui-<name>.
UI_ADDON_NAME_REGEX = re.compile(r"^[a-z0-9-]+$")


class VolumeSchema(Schema):
    name = fields.String(required=True)
    driver = fields.String(default="local")
    driver_opts = fields.Dict(keys=fields.String(), values=fields.String(), default={})
    labels = fields.Dict(keys=fields.String(), values=fields.String(), default={})


class NetworkSchema(Schema):
    name = fields.String(required=True)
    driver = fields.String(default="bridge")
    enable_ipv6 = fields.Boolean(default=False)


class ServiceSchema(Schema):
    service_name = fields.String(required=True)
    image = fields.String(required=True)
    command = fields.String()
    networks = fields.List(fields.String(), default=[])

    # e.g : ['/home/user1/:/mnt/vol2','/var/www:/mnt/vol1']
    volumes = fields.List(fields.String(), default=[])

    # e.g: {'2222:3333'}`` will expose port 2222 inside the container as port 3333 on the host.
    ports = fields.Dict(keys=fields.String(), values=fields.String(), default={})
    environment = fields.Dict(keys=fields.String(), values=fields.String(), default={})
    labels = fields.Dict(keys=fields.String(), values=fields.String(), default={})


class UiSchema(Schema):
    service = fields.String(required=True)
    sdk = fields.String(required=True, validate=validate.Length(min=1))


class MarketplaceAddonSchema(Schema):
    _id = fields.String()
    name = fields.String(required=True)
    description = fields.String()
    services = fields.Nested(ServiceSchema, many=True, required=True)
    status = fields.String(dump_only=True)
    volumes = fields.Nested(VolumeSchema, many=True, default=[])
    networks = fields.Nested(NetworkSchema, many=True, default=[])
    ui = fields.Nested(UiSchema)

    @validates_schema
    def validate_ui(self, data, **kwargs):
        ui = data.get("ui")
        if ui is None:
            return
        service_names = [service["service_name"] for service in data.get("services", [])]
        if ui["service"] not in service_names:
            raise ValidationError("ui.service must match one of the addon's services", "ui")
        if not UI_ADDON_NAME_REGEX.match(data.get("name", "")):
            raise ValidationError(
                "name must only contain lowercase letters, digits and hyphens for addons with a ui",
                "name",
            )


class MarketplaceFilterSchema(Schema):
    name = fields.String()

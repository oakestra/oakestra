from marshmallow import Schema, fields


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


class MarketplaceAddonSchema(Schema):
    _id = fields.String()
    name = fields.String(required=True)
    description = fields.String()
    services = fields.Nested(ServiceSchema, many=True, required=True)
    status = fields.String(dump_only=True)
    volumes = fields.Nested(VolumeSchema, many=True, default=[])
    networks = fields.Nested(NetworkSchema, many=True, default=[])


class MarketplaceFilterSchema(Schema):
    name = fields.String()

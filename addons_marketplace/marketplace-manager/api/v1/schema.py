from enum import Enum

from marshmallow import Schema, fields


class StatusEnum(Enum):
    UNDER_REVIEW = "under_review"
    APPROVED = "approved"


# TODO: support volumes
class ServiceSchema(Schema):
    service_name = fields.String(required=True)
    image_uri = fields.String(required=True)
    command = fields.String()
    networks = fields.List(fields.String(), default=[])
    ports = fields.Dict(keys=fields.String(), values=fields.String(), default={})
    environment = fields.Dict(keys=fields.String(), values=fields.String(), default={})
    labels = fields.Dict(keys=fields.String(), values=fields.String(), default={})


# TODO: support networks + volumes.
class MarketplaceAddonSchema(Schema):
    _id = fields.String()
    name = fields.String(required=True)
    description = fields.String()
    services = fields.Nested(ServiceSchema, many=True, required=True)
    status = fields.String(dump_only=True)


class MarketplaceFilterSchema(Schema):
    name = fields.String()

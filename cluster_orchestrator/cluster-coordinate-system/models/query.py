from marshmallow import Schema, fields
from dataclasses import dataclass


@dataclass
class QueryResponse:
    locations: dict


class QueryResponseSchema(Schema):
    locations = fields.Dict()

class QueryParameterSchema(Schema):
    ip_addresses = fields.Str()




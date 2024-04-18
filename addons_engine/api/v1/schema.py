from marshmallow import Schema, fields


class AddonSchema(Schema):
    _id = fields.String()
    marketplace_id = fields.String(required=True)
    status = fields.String(dump_only=True)


class AddonFilterSchema(Schema):
    status = fields.String()

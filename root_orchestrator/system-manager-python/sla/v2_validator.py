import jsonschema
from sla.schema import gateway_schema, sla_schema


def validate_json_v2(json_data):
    schema = {"oneOf": [sla_schema, gateway_schema]}
    try:
        jsonschema.validate(instance=json_data, schema=schema)
    except ValueError as err:
        print(err)
        return False
    except jsonschema.exceptions.ValidationError as err:
        print(err.message)
        return False
    return True

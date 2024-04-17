import jsonschema
from sla.schema import sla_schema


def validate_json_v2(json_data):
    try:
        jsonschema.validate(instance=json_data, schema=sla_schema)
    except ValueError as err:
        return err
    except jsonschema.exceptions.ValidationError as err:
        return err.message
    return None

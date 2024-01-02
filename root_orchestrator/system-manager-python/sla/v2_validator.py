import jsonschema
from sla.schema import sla_schema


def validate_json_v2(json_data):
    try:
        jsonschema.validate(instance=json_data, schema=sla_schema)
    except ValueError as err:
        print(err)
        return False
    except jsonschema.exceptions.ValidationError as err:
        print(err.message)
        return False
    return True

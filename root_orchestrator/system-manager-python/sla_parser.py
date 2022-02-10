import json
import jsonschema
from schema import sla_schema


class SLAFormatError(BaseException):
    message = "The given SLA was not formatted correctly"


def parse_sla(json_file):
    if validate_json(json_file):
        return json.loads(json_file)
    else:
        raise SLAFormatError


def validate_json(json_file):
    try:
        json_data = json.loads(json_file)
        jsonschema.validate(instance=json_data, schema=sla_schema)
    except ValueError as err:
        print(err)
        return False
    except jsonschema.exceptions.ValidationError as err:
        print(err.message)
        return False
    return True


if __name__ == "__main__":
    print(parse_sla(open("tests/service_level_agreements/sla_correct_1.json", 'r')))

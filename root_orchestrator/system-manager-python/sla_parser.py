import json
import jsonschema
from schema import sla_schema


class SLAFormatError(BaseException):
    message = "The given SLA was not formatted correctly"


def parse_sla(file):
    # json_file = open(file, "r").read()
    file_content = file.read()
    validate_json(file_content)
    return json.loads(file_content)

def read_from_file(file):
    json_file = open(file, "r").read()
    validate_json(json_file)
    return json.loads(json_file)

def validate_json(json_file):
    try:
        json_data = json.loads(json_file)
        jsonschema.validate(instance=json_data, schema=sla_schema)
    except ValueError as err:
        print(err)
        raise SLAFormatError
    except jsonschema.exceptions.ValidationError as err:
        print(err.message)
        raise SLAFormatError


if __name__ == "__main__":
    print(read_from_file("tests/service_level_agreements/sla_correct_1.json"))
import json
import logging

from sla.sla_versions import sla_validator_by_version


class SLAFormatError(BaseException):
    message = "The given SLA was not formatted correctly"


# if the file was sent with curl it is enough to use only file.read
# if it was uploaded with a post request we have to use also json.loads


def parse_sla_json(sla):
    json_data = sla
    if not isinstance(sla, dict):
        json_data = json.loads(sla)
    version = json_data["sla_version"]
    validator = sla_validator_by_version[version]
    validation_result = validator(json_data)
    if validation_result is None or validation_result is True:
        return json_data
    else:
        logging.log(logging.ERROR, validation_result)
        raise SLAFormatError(validation_result)

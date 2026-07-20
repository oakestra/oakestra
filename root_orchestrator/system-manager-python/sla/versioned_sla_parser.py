import json

from oakestra_logging import get_logger

from sla.sla_versions import sla_validator_by_version

logger = get_logger(__name__)


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
    logger.warning(
        "SLA validation failed",
        event_name="sla.validation.failed",
        sla_version=version,
        validation_error_count=(
            len(validation_result) if hasattr(validation_result, "__len__") else 1
        ),
    )
    raise SLAFormatError(validation_result)

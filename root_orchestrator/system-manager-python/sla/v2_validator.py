import jsonschema
from oakestra_logging import get_logger

from sla.schema import sla_schema

logger = get_logger(__name__)


def validate_json_v2(json_data):
    try:
        jsonschema.validate(instance=json_data, schema=sla_schema)
    except ValueError as err:
        logger.warning(
            "SLA validation could not complete",
            event_name="sla.validation.failed",
            sla_version="v2.0",
            validation_result={"reason": "Invalid validation input"},
        )
        return err
    except jsonschema.exceptions.ValidationError as err:
        logger.warning(
            "SLA validation failed",
            event_name="sla.validation.failed",
            sla_version="v2.0",
            validation_result={
                "reason": "Schema constraint violated",
                "constraint": err.validator,
                "expected": err.validator_value,
                "schema_path": list(err.absolute_schema_path),
            },
        )
        return err.message
    return None

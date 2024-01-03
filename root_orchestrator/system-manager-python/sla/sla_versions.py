from sla.v1_validator import yaml_reader
from sla.v2_validator import validate_json_v2

sla_validator_by_version = {"v1.0": yaml_reader, "v2.0": validate_json_v2}

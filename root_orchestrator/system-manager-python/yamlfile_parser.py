import os
import yaml
from cerberus import Validator
from schema import deploy_schema


def yaml_reader(file):
    """Load yaml file"""
    yaml_content = yaml.load(file, Loader=yaml.FullLoader)
    print(yaml_content.get('image'))
    yaml_validator(yaml_content)
    return yaml_content


def yaml_validator(yaml_file):
    print('validating yaml file...')
    script_dir = os.path.dirname(__file__)
    #schema_file_name = 'schema.py'
    #abs_file_path = os.path.join(script_dir, schema_file_name)
    #schema = eval(open(abs_file_path, 'r').read())
    v = Validator(deploy_schema)
    # TODO: propagate validation error
    print("validate: {0}".format(v.validate(yaml_file, deploy_schema)))
    print(v.errors)

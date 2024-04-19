import re

# Decorator to check if the sla is valid
def check_valid_sla(func):
    def check_sla(*args, **kwargs):
        if not valid_sla(kwargs.get("deployment_descriptor")):
            return "Invalid deployment scriptor", 400
        return func(*args, **kwargs)
    return check_sla

# Check if the sla is valid for the network component
def valid_sla(sla):
    if sla is None:
        return False
    if not valid_name(sla.get("app_name","")):
        return False
    if not valid_name(sla.get("app_ns","")):
        return False
    if not valid_name(sla.get("microservice_name","")):
        return False
    if not valid_name(sla.get("microservice_namespace","")):
        return False
    return True

# Check if only alphanumeric characters (min 1 max 30) are part of a given name
def valid_name(name):
    return bool(re.match(r'^[a-zA-Z0-9]{1,30}$', name))


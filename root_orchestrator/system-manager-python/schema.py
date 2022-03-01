deploy_schema = {
    'app_name': {
        'required': True,
        'type': 'string',
        'regex': '^[a-z]{2,12}'
    },
    'app_ns': {
        'required': True,
        'type': 'string',
        'regex': '^[a-z]{2,12}'
    },
    'service_name': {
        'required': True,
        'type': 'string',
        'regex': '^[a-z]{2,12}'
    },
    'service_ns': {
        'required': True,
        'type': 'string',
        'regex': '^[a-z]{2,12}'
    },
    'api_version': {
        'required': True,
        'type': 'string',
    },
    'RR_ip': {
        'required': False,
        'type': 'string',
    },
    'image': {
        'required': True,
        'type': 'string'
    },
    'requirements': {
        'required': True,
        'type': 'dict',
        'schema': {
            'cpu': {
                'required': True,
                'type': 'number'
            },
            'memory': {
                'required': True,
                'type': 'number'
            },
            'commands': {
                'required': False,
                'type': 'string'
            }
        }
    },
    'cluster': {
        'required': False,
        'type': 'dict',
        'schema': {
            'name': {
                'required': False,
                'type': 'string'
            },
            'node': {
                'required': False,
                'type': 'string'
            }
        }
    }
}

sla_schema = {
    "type": "object",
    "properties": {
        "api_version": {"type": "string"},
        "customerID": {"type": "integer"},
        "applications": {
            "type": "array",
            "items": {
                "type": "object",
                "properties": {
                    "applicationID": {"type": "integer"},
                    "application_name": {"type": "string"},
                    "application_namespace": {"type": "string"},
                    "application_desc": {"type": "string"},
                    "microservices": {
                        "type": "array",
                        "items": {
                            "type": "object",
                            "properties": {
                                "microserviceID": {"type": "integer"},
                                "microservice_name": {"type": "string"},
                                "microservice_namespace": {"type": "string"},
                                "virtualization": {"type": "string"},
                                "memory": {"type": "integer"},
                                "vcpus": {"type": "integer"},
                                "vgpus": {"type": "integer"},
                                "vtpus": {"type": "integer"},
                                "bandwidth_in": {"type": "integer"},
                                "bandwith_out": {"type": "integer"},
                                "storage": {"type": "integer"},
                                "code": {"type": "string"},
                                "state": {"type": "string"},
                                "port": {"type": "integer"},
                                "addresses": {
                                    "type": "object",
                                    "properties": {
                                        "rr_ip": {"type": "string"},
                                        "closest_ip": {"type": "string"},
                                        "instances": {
                                            "type": "array",
                                            "items": {
                                                "type": "object",
                                                "properties": {
                                                    "from": {"type": "string"},
                                                    "to": {"type": "string"},
                                                    "start": {"type": "string"},
                                                }
                                            }
                                        },
                                    }
                                },
                                "added_files": {
                                    "type": "array",
                                    "items": {
                                        "type": "string",
                                    }
                                },
                                "args": {
                                    "type": "array",
                                    "items": {
                                        "type": "string",
                                    }
                                },
                                "constraints": {
                                    "type": "array",
                                    "items": {
                                        "type": "object",
                                        "properties": {
                                            "type": {"type": "string"},
                                            "area": {"type": "string"},
                                            "location": {"type": "string"},
                                            "threshold": {"type": "number"},
                                            "rigidness": {"type": "number"},
                                            "convergence_time": {"type": "integer"},
                                        },
                                        "required": ["type", "threshold", "rigidness", "convergence_time"]
                                    }
                                },
                                "connectivity": {
                                    "type": "array",
                                    "items": {
                                        "type": "object",
                                        "properties": {
                                            "target_microservice_id": {"type": "integer"},
                                            "con_constraints": {
                                                "type": "array",
                                                "items": {
                                                    "type": "object",
                                                    "properties": {
                                                        "type": {"type": "string"},
                                                        "threshold": {"type": "number"},
                                                        "rigidness": {"type": "number"},
                                                        "convergence_time": {"type": "integer"},
                                                    },
                                                    "required": ["type", "threshold", "rigidness", "convergence_time"]
                                                }
                                            },
                                        },
                                        "required": ["target_microservice_id", "con_constraints"]
                                    }
                                },
                            },
                            "required": ["microserviceID", "microservice_name", "virtualization", "memory", "vcpus",
                                         "vgpus", "vtpus", "bandwidth_in",  "bandwith_out", "storage", "code", "state",
                                         "port", "added_files", "constraints", "connectivity"]
                        },
                        "exclusiveMinimum": 0,
                    },
                },
                "required": ["applicationID", "application_name", "application_desc", "microservices"],
            }
        },
        "args": {
            "type": "array",
            "items": {
                "type": "string",
            }
        },
    },
    "required": ["api_version", "customerID", "applications"]
}

deploy_schema = {
    'customerID': {
        'required': True,
        'type': 'string'
    },
    'args': {
        'required': False,
        'type': 'string'
    },
    'applications': {
        'required': True,
        'type': 'list',
        'schema': {
            'type': 'dict',
            'schema': {
                'applicationID': {
                    'required': True,
                    'type': 'string',
                },
                'app_name': {
                    'required': True,
                    'type': 'string'
                },
                'app_ns': {
                    'required': True,
                    'type': 'string'
                },
                'application_desc': {
                    'required': False,
                    'type': 'string'
                },
                'microservices': {
                    'required': True,
                    'type': 'list',
                    'schema': {
                        'type': 'dict',
                        'schema': {
                            'microserviceID': {
                                'required': True,
                                'type': 'string',
                            },
                            'service_name': {
                                'required': True,
                                'type': 'string'
                            },
                            'service_ns': {
                                'required': True,
                                'type': 'string'
                            },
                            'virtualization': {
                                'required': True,
                                'type': 'string'
                            },
                            'memory': {
                                'required': True,
                                'type': 'number'
                            },
                            'vcpus': {
                                'required': True,
                                'type': 'number'
                            },
                            'cgpus': {
                                'required': False,
                                'type': 'number'
                            },
                            'vtpus': {
                                'required': False,
                                'type': 'number'
                            },
                            'bandwidth_in': {
                                'required': False,
                                'type': 'number'
                            },
                            'bandwidth_out': {
                                'required': False,
                                'type': 'number'
                            },
                            'storage': {
                                'required': False,
                                'type': 'number'
                            },
                            # TODO: code or image?
                            'code': {
                                'required': True,
                                'type': 'string'
                            },
                            # TODO: check why port is missing -> required in container startup in worker node
                            'port': {
                                'required': False,
                                'type': 'number'
                            },
                            'constraints': {
                                'required': False,
                                'type': 'list',
                                'schema': {
                                    'type': 'dict',
                                    'schema': {
                                        'type': {
                                            'required': True,
                                            'type': 'string',
                                            'allowed': ['latency', 'geo']
                                        },
                                        'area': {
                                            'type': 'string',
                                            'dependencies': {
                                                'type': ['latency']
                                            }
                                        },
                                        'location': {
                                            'type': 'string',
                                            'dependencies': {
                                                'type': ['geo']
                                            }
                                        },
                                        'threshold': {
                                            'type': 'number',
                                            'dependencies': {
                                                'type': ['latency', 'geo']
                                            }
                                        },
                                        'rigidness': {
                                            'type': 'number',
                                            'dependencies': {
                                                'type': ['latency', 'geo']
                                            }
                                        },
                                    }
                                }
                            },
                            'connectivity': {
                                'required': False,
                                'type': 'list',
                                'schema': {
                                    'type': 'dict',
                                    'schema': {
                                        'target_microservice_id': {
                                            'required': True,
                                            'type': 'string'
                                        },
                                        'con_constraints': {
                                            'required': False,
                                            'type': 'list',
                                            'schema': {
                                                'type': 'dict',
                                                'schema': {
                                                    'type': {
                                                        'required': True,
                                                        'type': 'string',
                                                        'allowed': ['latency', 'geo']
                                                    },
                                                    'threshold': {
                                                        'type': 'number',
                                                        'required': True
                                                    },
                                                    'rigidness': {
                                                        'type': 'number',
                                                        'required': True
                                                    }
                                                }
                                            }
                                        }
                                    }
                                }
                            }
                        }

                    }
                }
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
                                "bandwidth_out": {"type": "integer"},
                                "storage": {"type": "integer"},
                                "code": {"type": "string"},
                                "state": {"type": "string"},
                                "port": {"type": "string"},
                                "sla_violation_strategy": {"type": "string"},
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
                                         "vgpus", "vtpus", "bandwidth_in",  "bandwidth_out", "storage", "code", "state",
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

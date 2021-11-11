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

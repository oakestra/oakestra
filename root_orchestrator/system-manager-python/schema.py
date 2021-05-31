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

{
    'api_version': {
        'required': True,
        'type': 'string'
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

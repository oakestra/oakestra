import os

from flask import Flask
from app.extensions.mqtt_client import mqtt as flask_mqtt
from app.extensions.celery import celery

def create_app(logger=True):
    """
    Application factory
    Using this design pattern, no application-specific state is stored on the extension object, so one extension
    object can be used for multiple apps.
    """

    app = Flask(__name__)
    # setup with the configuration provided by the user / environment
    app.config.from_object("config")

    if not logger:
        from flask.logging import default_handler
        app.logger.removeHandler(default_handler)

    # initialize Vivaldi coordinates
    from app.models.vivaldi_coordinate import VivaldiCoordinate
    vivaldi_dim = int(os.environ.get("VIVALDI_DIM"))
    app.config['VIVALDI_COORDS'] = VivaldiCoordinate(vivaldi_dim)

    app.config['MQTT_BROKER_URL'] = os.environ.get("CLUSTER_MANAGER_IP")
    app.config['MQTT_REFRESH_TIME'] = 3.0  # refresh time in seconds
    # MQTT port is required for monitoring component to send alarm via (Paho) MQTT
    # Paho MQTT because celery task is not within a flask context and hence cannot use flask mqtt context
    app.config['MQTT_BROKER_PORT'] = int(os.environ.get('MQTT_BROKER_PORT'))
    flask_mqtt.init_app(app)
    flask_mqtt.app = app

    # configure celery
    app.config['CELERY_BROKER_URL'] = os.environ.get('REDIS_ADDR')
    init_celery(app)

    # init_celery(app)

    # configure logging
    from app.extensions.logging import configure_logging
    my_logger = configure_logging(__name__)

    # register blueprints
    from app.blueprints.monitoring.monitoring import monitoring
    app.register_blueprint(monitoring)
    from app.blueprints.node_engine.node_engine import node_engine
    app.register_blueprint(node_engine)
    from app.blueprints.network_measurement.network_measurement import network_measurement
    app.register_blueprint(network_measurement)

    return app


def init_celery(app=None):
    # TODO: check possibility to initialize paho mqtt here in order to be able to use same mqtt instance for the whole flask app including celery task contexts
    app = app or create_app(logger=False)
    celery.conf.broker_url = app.config["CELERY_BROKER_URL"]
    celery.conf.result_backend = app.config["CELERY_BROKER_URL"]
    celery.conf.update(app.config)

    class ContextTask(celery.Task):
        """ Make celery tasks work with Flask app context """
        def __call__(self, *args, **kwargs):
            with app.app_context():
                return self.run(*args, **kwargs)

    celery.Task = ContextTask
    return celery
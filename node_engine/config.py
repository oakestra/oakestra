import os

class Config(object):
    """
    Base configuration class. Contains default configuration settings + configuration settings applicable to all environments
    """
    DEBUG = False
    TESTING = False

    CELERY_BROKER_URL = os.environ.get('REDIS_ADDR')

class ProductionConfig(Config):
    DEBUG = False

class DevelopmentConfig(Config):
    ENV="development"
    DEVELOPMENT=True
    DEBUG=True


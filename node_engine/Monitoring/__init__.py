from celery import Celery
REDIS_ADDR="redis://:workerRedis@localhost:6380"
celeryapp = Celery(__name__, backend=REDIS_ADDR, broker=REDIS_ADDR)
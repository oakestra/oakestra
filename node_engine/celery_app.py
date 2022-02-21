from app import init_celery
from app.blueprints.node_engine.node_engine import init_node_engine

app = init_celery()
app.conf.imports = app.conf.imports + ("app.blueprints.monitoring.monitoring_tasks",)

from app import celery
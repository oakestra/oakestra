import json
from flask import Blueprint, request

from app.blueprints.network_measurement.network_measurement import parallel_ping_retry
from app.blueprints.monitoring.monitoring_tasks import monitor_docker_container
from app.extensions.logging import configure_logging
my_logger = configure_logging("monitoring")
monitoring = Blueprint("monitoring", __name__, url_prefix="/monitoring")

@monitoring.route("/")
def hello_world():
    my_logger.info('Hello World Request')
    return "Hello, World! This is the monitoring component's REST API"


@monitoring.route('/ping', methods=['POST'])
def ping():
    my_logger.info(f"Incoming Request /ping Body: {request.json}")
    target_ips = json.loads(request.json)
    statistics = parallel_ping_retry(target_ips)

    print(f"Ping stats: {statistics}")
    return statistics, 200

@monitoring.route('/register', methods=['POST'])
def handle_register_service():
    data = json.loads(request.json)
    my_logger.info(f"Incoming Request /register Body: {data}")
    job = data.get("job")
    node_id = data.get("node_id")
    container_id = data.get("container_id")
    container_port = data.get("port")
    monitor_docker_container.delay(job, container_id, container_port, node_id)
    return "ok", 200

def register_service(node_id, container_id, port, job):
    my_logger.info("Incoming Request - Register service for monitoring...")
    monitor_docker_container.delay(job, container_id, port, node_id)
    return "ok", 200




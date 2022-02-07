import os
import time

from flask import Flask, request
from celery import Celery
import json
from mongodb_client import mongo_init
from manager_requests import manager_request
from calculation import calculate
from cs_logging import configure_logging

WORKER_SCREENING_INTERVAL = 30

MY_PORT = os.environ.get("MY_PORT")

my_logger = configure_logging()

app = Flask(__name__)

REDIS_ADDR = os.environ.get('REDIS_ADDR')
celeryapp = Celery('cluster_scheduler', backend=REDIS_ADDR, broker=REDIS_ADDR)

mongo_init(app)


@app.route('/')
def hello_world():
    app.logger.info("Hello World Request")
    return "Hello, World! This is the Cluster_Scheduler.\n"


@app.route('/status')
def status():
    return "ok"


@app.route('/test/celery')
def test_celery():
    app.logger.info('Request /test/celery')
    test_celery.delay()
    return "ok", 200


@app.route('/api/calculate/deploy', methods=['GET', 'POST'])
def deploy_task():
    app.logger.info('Request /api/calculate/deploy\n')

    job = request.json  # contains job_id and job_description
    app.logger.info(job)
    start_calc_deploy.delay(job)
    return "ok"


@app.route('/api/calculate/replicate', methods=['GET', 'POST'])
def replicate_task():
    app.logger.info('Request /api/calculate/replicate\n')

    data = request.json  # contains job_id and job_description
    job = data.get('job')
    app.logger.info(job)
    start_calc_replicate.delay(job)
    return "ok"

@app.route("/api/calculate/sla-alarm", methods=["POST"])
def handle_sla_alarm():
    data = request.json
    topic = data.get('topic')
    payload = data.get('payload')
    client_id = topic.split('/')[1]
    print(f"Payload {payload}")
    handle_sla_alarm_task.delay(client_id, payload)

    return "ok", 204

@celeryapp.task
def handle_sla_alarm_task(client_id, payload):
    print(f"Payload {payload}")
    job = payload.get("job")
    ip_rtt_stats = payload.get("ip_rtt_stats")
    # Deploy service to new target
    scheduling_status, scheduling_result, augmented_job = calculate(job, is_sla_violation=True, source_client_id=client_id, worker_ip_rtt_stats=ip_rtt_stats)
    # Undeploy service on violating node
    if scheduling_status == 'negative':
        app.logger.info('No active node found to schedule this job.')
    else:
        app.logger.info('Chosen Node: {0}'.format(str(scheduling_result.get("_id"))))
        manager_request(app, scheduling_result, augmented_job)

@celeryapp.task()
def start_calc_deploy(job):
    # i = celeryapp.control.inspect()
    # print(i)
    app.logger.info("App.logger.info Received Task")
    print("print Received Task")

    scheduling_status, scheduling_result, augmented_job = calculate(job)  # scheduling_result can be a node object

    if scheduling_status == 'negative':
        app.logger.info('No active node found to schedule this job.')
    else:
        app.logger.info('Chosen Node: {0}'.format(str(scheduling_result.get("_id"))))
        manager_request(app, scheduling_result, augmented_job)
        # mongo_set_job_as_scheduled(job_id=job.get('_id'), node_id=scheduling_result.get('_id')) # DONE IN CLUSTER-MANAGER


@celeryapp.task()
def start_calc_replicate(job):
    print(job)

    scheduling_status, scheduling_result, augmented_job = calculate(job)

    if scheduling_status == 'negative':
        app.logger.info("Target node does not provide the required resources.")
    else:
        app.logger.info(f'Send scheduling result for node {scheduling_result}')
        manager_request(app, scheduling_result, augmented_job)


@celeryapp.on_after_configure.connect
def setup_periodic_tasks(sender, **kwargs):
    #  every INTERVAL, execute screen_worker_nodes() to look for dead worker nodes
    sender.add_periodic_task(WORKER_SCREENING_INTERVAL, screen_worker_nodes.s('hello'), name='screen worker nodes')


@celeryapp.task
def screen_worker_nodes(arg):
    app.logger.info(arg)
    # Look into database: search for not reporting worker nodes (not reporting = dead)
    # Look if those worker nodes have any applications deployed
    # calculate new placements for the


@celeryapp.task()
def test_celery():
    app.logger.info("Celery test method")


if __name__ == '__main__':
    app.run(debug=False, host='0.0.0.0', port=MY_PORT)

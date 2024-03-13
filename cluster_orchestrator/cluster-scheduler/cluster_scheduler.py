import os

from calculation import calculate
from celery import Celery
from cs_logging import configure_logging
from flask import Flask, request
from manager_requests import manager_request
from mongodb_client import mongo_init

WORKER_SCREENING_INTERVAL = 30

MY_PORT = os.environ.get("MY_PORT")

my_logger = configure_logging()

app = Flask(__name__)

REDIS_ADDR = os.environ.get("REDIS_ADDR")
celeryapp = Celery("cluster_scheduler", backend=REDIS_ADDR, broker=REDIS_ADDR)

mongo_init(app)


@app.route("/")
def hello_world():
    app.logger.info("Hello World Request")
    return "Hello, World! This is the Cluster_Scheduler.\n"


@app.route("/status")
def status():
    return "ok"


@app.route("/test/celery")
def test_celery():
    app.logger.info("Request /test/celery")
    test_celery.delay()
    return "ok", 200


@app.route("/api/calculate/deploy/<job_id>/<instance_num>", methods=["GET", "POST"])
def deploy_task(job_id, instance_num):
    app.logger.info("Request /api/calculate/deploy\n")

    job = request.json  # contains job_id and job_description
    app.logger.info(job)
    start_calc_deploy.delay(job, job_id, instance_num)
    return "ok"


@app.route("/api/calculate/replicate", methods=["GET", "POST"])
def replicate_task():
    app.logger.info("Request /api/calculate/replicate\n")

    job = request.json  # contains job_id and job_description
    app.logger.info(job)
    start_calc_replicate.delay(job)
    return "ok"


@celeryapp.task()
def start_calc_deploy(job, job_id, instance_num):
    # i = celeryapp.control.inspect()
    # print(i)
    app.logger.info("App.logger.info Received Task")
    print("print Received Task")

    scheduling_status, scheduling_result = calculate(
        app, job
    )  # scheduling_result can be a node object

    if scheduling_status == "negative":
        app.logger.info("No active node found to schedule this job.")
        manager_request(app, None, job, job_id, instance_num)
    else:
        app.logger.info("Chosen Node: {0}".format(scheduling_result))
        manager_request(app, scheduling_result, job, job_id, instance_num)
        # DONE IN CLUSTER-MANAGER
        # mongo_set_job_as_scheduled(job_id=job.get('_id'), node_id=scheduling_result.get('_id'))


@celeryapp.task()
def start_calc_replicate(job):
    print(job)


@celeryapp.on_after_configure.connect
def setup_periodic_tasks(sender, **kwargs):
    #  every INTERVAL, execute screen_worker_nodes() to look for dead worker nodes
    sender.add_periodic_task(
        WORKER_SCREENING_INTERVAL,
        screen_worker_nodes.s("hello"),
        name="screen worker nodes",
    )


@celeryapp.task
def screen_worker_nodes(arg):
    app.logger.info(arg)
    # Look into database: search for not reporting worker nodes (not reporting = dead)
    # Look if those worker nodes have any applications deployed
    # calculate new placements for the


# @celeryapp.task()
# def test_celery():
#     app.logger.info("Celery test method")


if __name__ == "__main__":
    app.run(debug=False, host="::", port=MY_PORT)

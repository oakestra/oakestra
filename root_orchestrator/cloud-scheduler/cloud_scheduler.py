import os

from calculation import calculate
from celery import Celery
from cs_logging import configure_logging
from flask import Flask, request
from manager_requests import manager_request
from resource_management import job_operations

CLUSTER_SCREENING_INTERVAL = 60

MY_PORT = os.environ.get("MY_PORT")

my_logger = configure_logging()

app = Flask(__name__)

REDIS_ADDR = os.environ.get("REDIS_ADDR")
celeryapp = Celery("cloud_scheduler", backend=REDIS_ADDR, broker=REDIS_ADDR)


@app.route("/")
def hello_world():
    return "Hello, World! This is the Cloud_Scheduler.\n"


@app.route("/status")
def status():
    return "ok"


@app.route("/test/celery")
def test_celery():
    app.logger.info("Request /test/celery")
    test_celery.delay()
    return "ok", 200


@app.route("/api/calculate/deploy", methods=["GET", "POST"])
def deploy_task():
    print("request /api/calculate\n")
    data = request.json
    job = data["job"]
    job_id = data["system_job_id"]
    start_calc.delay(job_id, job)
    return "ok"


#  @celeryapp.on_after_configure.connect
#  def setup_periodic_tasks(sender, **kwargs):
# Calls test('hello') every 10 seconds.
# sender.add_periodic_task(
#     CLUSTER_SCREENING_INTERVAL,
#     cluster_screening.s("hello"),
#     name="screen clusters",
# )


@celeryapp.task
def cluster_screening(arg):
    app.logger.info(arg)


@celeryapp.task()
def start_calc(job_id, job):
    # i = celeryapp.control.inspect()
    # print(i)

    scheduling_status, scheduling_result = calculate(job_id, job)
    print(scheduling_result)
    if scheduling_status == "negative":
        job_operations.update_job_status(job_id, scheduling_result)
    else:
        scheduling_result.get("_id")
        # mongo_update_job_status_and_cluster(job_id, 'CLUSTER_SCHEDULED', cluster_id)
        manager_request(
            scheduling_result, job_id, job, replicas=1
        )  # scheduling_result is a target cluster


# @celeryapp.task()
# def test_celery():
#     app.logger.info("Celery test method")


if __name__ == "__main__":
    app.run(debug=False, host="0.0.0.0", port=MY_PORT)

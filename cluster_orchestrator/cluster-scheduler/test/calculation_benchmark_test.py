import csv
import os
import sys
import time
from unittest.mock import MagicMock

import calculation
from oakestra_utils.types.statuses import NegativeSchedulingStatus

myPath = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, myPath + "/../")

results = [["test_n", "component", "overhead", "setup"]]


def db_gen(num):
    cluster_list = []
    for i in range(num):
        elem = {
            "current_cpu_cores_free": 12,
            "current_free_memory_in_MB": 230,
            "_id": str(i),
            "node_info": {"technology": ["docker"]},
        }
        cluster_list.append(elem)
    return cluster_list


def run_test(
    current_setup_first_number: int,
    current_setup_second_number: int,
):
    calculation.mongo_find_all_active_nodes = MagicMock(
        return_value=db_gen(current_setup_second_number)
    )
    job = {
        "requirements": {
            "cpu": 1,
            "memory": 100,
        },
        "id": 1,
        "image_runtime": "docker",
    }
    for i in range(100):
        app = MagicMock()
        app.logger.info = lambda x: None
        start = time.time()
        res = calculation.calculate(app, job)
        if isinstance(res, NegativeSchedulingStatus):
            raise Exception()
        stop = time.time()
        overhead = stop - start
        overhead *= 1000
        results.append(
            [i, "cluster", overhead, f"{current_setup_first_number}-{current_setup_second_number}"]
        )

    print_csv()


def test_banchemark_45():
    run_test(1, 45)


def test_banchemark_15():
    run_test(3, 15)


def test_banchemark_9():
    run_test(5, 9)


def test_banchemark_5():
    run_test(9, 5)


def test_banchemark_3():
    run_test(15, 3)


def test_banchemark_1():
    run_test(45, 1)


def print_csv():
    with open("results-scheduler.csv", "w+") as my_csv:
        csvWriter = csv.writer(my_csv, delimiter=",")
        csvWriter.writerows(results)

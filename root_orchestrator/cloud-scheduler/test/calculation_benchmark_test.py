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
        elem = {"total_cpu_cores": 12, "memory_in_mb": 230, "_id": str(i)}
        cluster_list.append(elem)
    return cluster_list


def run_test(
    current_setup_first_number: int,
    current_setup_second_number: int,
):
    calculation.mongo_find_all_active_clusters = MagicMock(
        return_value=db_gen(current_setup_first_number)
    )
    job = {
        "requirements": {
            "cpu": 1,
            "memory": 100,
        },
        "id": 1,
    }
    for i in range(100):
        start = time.time()
        res = calculation.calculate(job)
        if isinstance(res, NegativeSchedulingStatus):
            raise Exception()
        stop = time.time()
        overhead = stop - start
        overhead *= 1000
        results.append(
            [i, "root", overhead, f"{current_setup_first_number}-{current_setup_second_number}"]
        )

    print_csv()


def test_banchemark_1():
    run_test(1, 45)


def test_banchemark_3():
    run_test(3, 15)


def test_banchemark_5():
    run_test(5, 9)


def test_banchemark_9():
    run_test(9, 5)


def test_banchemark_15():
    run_test(15, 3)


def test_banchemark_45():
    run_test(45, 1)


def print_csv():
    with open("results-scheduler.csv", "w+") as my_csv:
        csvWriter = csv.writer(my_csv, delimiter=",")
        csvWriter.writerows(results)

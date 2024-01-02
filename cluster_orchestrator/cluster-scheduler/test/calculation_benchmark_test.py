import csv
import os
import sys
import time
from unittest.mock import MagicMock

import calculation

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


def test_banchemark_45():
    current_setup = "1-45"
    calculation.mongo_find_all_active_nodes = MagicMock(return_value=db_gen(45))
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
        res, mes = calculation.calculate(app, job)
        if res != "positive":
            raise Exception()
        stop = time.time()
        overhead = stop - start
        overhead *= 1000
        results.append([i, "cluster", overhead, current_setup])

    print_csv()


def test_banchemark_15():
    current_setup = "3-15"
    calculation.mongo_find_all_active_nodes = MagicMock(return_value=db_gen(15))
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
        res, mes = calculation.calculate(app, job)
        if res != "positive":
            raise Exception()
        stop = time.time()
        overhead = stop - start
        overhead *= 1000
        results.append([i, "cluster", overhead, current_setup])

    print_csv()


def test_banchemark_9():
    current_setup = "5-9"
    calculation.mongo_find_all_active_nodes = MagicMock(return_value=db_gen(9))
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
        res, mes = calculation.calculate(app, job)
        if res != "positive":
            raise Exception()
        stop = time.time()
        overhead = stop - start
        overhead *= 1000
        results.append([i, "cluster", overhead, current_setup])

    print_csv()


def test_banchemark_5():
    current_setup = "9-5"
    calculation.mongo_find_all_active_nodes = MagicMock(return_value=db_gen(5))
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
        res, mes = calculation.calculate(app, job)
        if res != "positive":
            raise Exception()
        stop = time.time()
        overhead = stop - start
        overhead *= 1000
        results.append([i, "cluster", overhead, current_setup])

    print_csv()


def test_banchemark_3():
    current_setup = "15-3"
    calculation.mongo_find_all_active_nodes = MagicMock(return_value=db_gen(3))
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
        res, mes = calculation.calculate(app, job)
        if res != "positive":
            raise Exception()
        stop = time.time()
        overhead = stop - start
        overhead *= 1000
        results.append([i, "cluster", overhead, current_setup])

    print_csv()


def test_banchemark_1():
    current_setup = "45-1"
    calculation.mongo_find_all_active_nodes = MagicMock(return_value=db_gen(1))
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
        res, mes = calculation.calculate(app, job)
        if res != "positive":
            raise Exception()
        stop = time.time()
        overhead = stop - start
        overhead *= 1000
        results.append([i, "cluster", overhead, current_setup])

    print_csv()


def print_csv():
    with open("results-scheduler.csv", "w+") as my_csv:
        csvWriter = csv.writer(my_csv, delimiter=",")
        csvWriter.writerows(results)

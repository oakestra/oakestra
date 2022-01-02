import sys, os
import csv
import time
from unittest.mock import MagicMock

from util import db_gen

myPath = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, myPath + '/../')

import calculation

results = [["test_n", "component", "overhead", "setup"]]


def test_banchemark_3():
    current_setup = "3-15"
    calculation.mongo_find_all_active_clusters = MagicMock(return_value=db_gen(3))
    job = {
        'requirements': {
            "cpu": 1,
            "memory": 100,
        },
        "id": 1
    }
    for i in range(100):
        start = time.time()
        res, mes = calculation.calculate("1", job)
        if res != "positive":
            raise Exception()
        stop = time.time()
        overhead = stop - start
        overhead *= 1000
        results.append([i, "root", overhead, current_setup])

    print_csv()


def test_banchemark_5():
    current_setup = "5-9"
    calculation.mongo_find_all_active_clusters = MagicMock(return_value=db_gen(5))
    job = {
        'requirements': {
            "cpu": 1,
            "memory": 100,
        },
        "id": 1
    }
    for i in range(100):
        start = time.time()
        res, mes = calculation.calculate("1", job)
        if res != "positive":
            raise Exception()
        stop = time.time()
        overhead = stop - start
        overhead *= 1000
        results.append([i, "root", overhead, current_setup])

    print_csv()


def test_banchemark_9():
    current_setup = "9-5"
    calculation.mongo_find_all_active_clusters = MagicMock(return_value=db_gen(9))
    job = {
        'requirements': {
            "cpu": 1,
            "memory": 100,
        },
        "id": 1
    }
    for i in range(100):
        start = time.time()
        res, mes = calculation.calculate("1", job)
        if res != "positive":
            raise Exception()
        stop = time.time()
        overhead = stop - start
        overhead *= 1000
        results.append([i, "root", overhead, current_setup])

    print_csv()


def test_banchemark_15():
    current_setup = "15-3"
    calculation.mongo_find_all_active_clusters = MagicMock(return_value=db_gen(15))
    job = {
        'requirements': {
            "cpu": 1,
            "memory": 100,
        },
        "id": 1
    }
    for i in range(100):
        start = time.time()
        res, mes = calculation.calculate("1", job)
        if res != "positive":
            raise Exception()
        stop = time.time()
        overhead = stop - start
        overhead *= 1000
        results.append([i, "root", overhead, current_setup])

    print_csv()


def test_banchemark_45():
    current_setup = "45-1"
    calculation.mongo_find_all_active_clusters = MagicMock(return_value=db_gen(45))
    job = {
        'requirements': {
            "cpu": 1,
            "memory": 100,
        },
        "id": 1
    }
    for i in range(100):
        start = time.time()
        res, mes = calculation.calculate("1", job)
        if res != "positive":
            raise Exception()
        stop = time.time()
        overhead = stop - start
        overhead *= 1000
        results.append([i, "root", overhead, current_setup])

    print_csv()


def print_csv():
    with open("results-scheduler.csv", "w+") as my_csv:
        csvWriter = csv.writer(my_csv, delimiter=',')
        csvWriter.writerows(results)

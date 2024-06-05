import argparse
import csv
import json
import os
import random
import threading
import time
from random import randint

import requests
from flask import Flask
from icecream import ic

SYSTEM_MANAGER_URL = os.getenv("SYSTEM_MANAGER_URL", "0.0.0.0:10000")
ADDONS_ENGINE_URL = os.getenv("ADDONS_ENGINE_URL", "0.0.0.0:11101")
ADDONS_MARKETPLACE_URL = os.getenv("ADDONS_MARKETPLACE_URL", "0.0.0.0:11102")

app = Flask(__name__)
start = None
stop = None
lock = False
last_service = None
service_count = 0

results = []
results.append(["test_n", "time"])


def get_images_list():
    small_images = [
        "docker.io/library/busybox:latest",  # ~1.2MB
        "docker.io/library/alpine:latest",  # ~5MB
        "docker.io/library/node:14-alpine",  # ~39MB
        "docker.io/library/python:3.9-alpine",  # ~42MB
    ]

    medium_images = [
        "memcached",  # ~75MB
        "redis",  # ~104MB
        "nginx",  # ~133MB
        "httpd",  # ~166MB
        "rabbitmq",  # ~182MB
    ]

    large_images = [
        "mcr.microsoft.com/dotnet/core/aspnet:3.1",  # ~207MB
        "golang:1.16-alpine",  # ~299MB
        "postgres",  # ~314MB
        "mongo",  # ~493MB
        "mcr.microsoft.com/windows/servercore:ltsc2019",  # ~5GB
    ]

    return [small_images, medium_images, large_images]


def get_gio_image():
    return "docker.io/giobart/edgeio-deployment-time-script:deployment-edgeio"


def get_random_image(lightweight=True):
    images = get_images_list()

    return random.choice(images[0] if lightweight else random.choice(images))


def get_random_addon(lightweight=True):
    service_name = get_random_string(6)
    image = get_random_image(lightweight)

    return {
        "name": get_random_string(6),
        "services": [
            {
                "service_name": service_name,
                "image": image,
                "command": "/bin/sh -c 'while true; do echo \"Hello, World (testing!!) !\"; sleep 10; done'",  # noqa
                "ports": {},
                "environment": {},
                "networks": [],
            }
        ],
    }


def get_random_string(length):
    # choose from all lowercase letter
    result_str = "".join(random.choice("qwertyuiopasdfghjklzxcvbnm") for i in range(length))
    return result_str


def get_fake_sla_app(services=None, namespace="test"):
    microservice_list = services or []

    return {
        "sla_version": "v2.0",
        "customerID": "Admin",
        "applications": [
            {
                "application_name": get_random_string(5),
                "application_namespace": namespace,
                "application_desc": "No description here",
                "applicationID": "",
                "microservices": microservice_list,
            }
        ],
    }


def get_fake_sla_service(
    name,
    namespace="test",
    image="",
    virt="container",
    environment=[],
    cmd=None,
):

    result = {
        "microserviceID": "",
        "microservice_name": name,
        "microservice_namespace": namespace,
        "virtualization": virt,
        "memory": 0,
        "vcpus": 0,
        "vgpus": 0,
        "vtpus": 0,
        "bandwidth_in": 0,
        "bandwidth_out": 0,
        "storage": 0,
        "code": image,
        "port": "80",
        "one_shot": True,
        "environment": environment,
    }

    if cmd:
        result["cmd"] = cmd

    return result


def get_full_random_sla_app(server_address=None):
    namespace = get_random_string(4)
    app = get_fake_sla_app(namespace=namespace)
    name = get_random_string(4)

    for i in range(randint(1, 2)):
        image = get_gio_image()
        namespace = get_random_string(4)
        service = get_fake_sla_service(
            name + str(i),
            namespace=namespace,
            image=image,
            virt="container",
            environment=[f"SERVER_ADDRESS={server_address}"] if server_address else [],
            cmd=(
                None
                if server_address
                else [
                    "/bin/sh",
                    "-c",
                    "while true; do echo 'Hello, World (testing)'; sleep 10; done",
                ]
            ),
        )
        app["applications"][0]["microservices"].append(service)

    return app


def get_first_app(apps):
    return apps["applications"][0]


def get_login():
    url = "http://" + SYSTEM_MANAGER_URL + "/api/auth/login"
    credentials = {"username": "Admin", "password": "Admin"}
    r = requests.post(url, json=credentials)
    return r.json()["token"]


def register_app(sla):
    token = get_login()
    ic(sla)

    head = {"Authorization": "Bearer " + token}
    url = "http://" + SYSTEM_MANAGER_URL + "/api/application"

    resp = requests.post(url, headers=head, json=sla)
    if resp.status_code >= 400:
        ic("Failed to register app.")
        exit(1)

    result = json.loads(resp.json())
    ic(result)

    return result


def delete_all_apps():
    token = get_login()
    url = "http://" + SYSTEM_MANAGER_URL + "/api/applications"
    response = requests.get(url, headers={"Authorization": "Bearer {}".format(token)})

    if response.status_code >= 400:
        ic("Failed to get apps.")
        exit(1)

    apps = json.loads(response.json())
    for app in apps:
        ic(f"deleting app...{app.get('applicationID', '')}")

        app_id = app.get("applicationID")
        url = f"http://{SYSTEM_MANAGER_URL}/api/application/{app_id}"
        response = requests.delete(url, headers={"Authorization": "Bearer {}".format(token)})

        if response.status_code >= 400:
            ic("Failed to delete app.")


def deploy(service_id):
    global start

    ic(f"Deploying service...{str(service_id)}")

    token = get_login()
    url = f"http://{SYSTEM_MANAGER_URL}/api/service/{service_id}/instance"
    start = time.time()

    resp = requests.post(url, headers={"Authorization": "Bearer {}".format(token)})
    if resp.status_code >= 400:
        ic("Deploy request failed!")
        exit(1)


def stress_app_test(apps_count=10, server_address=None):
    global lock, last_service

    for i in range(apps_count):
        ic("Registering app..." + str(i))
        sla = get_full_random_sla_app(server_address)
        user_apps = register_app(sla)
        app = next(
            (
                item
                for item in user_apps
                if item.get("application_name") == get_first_app(sla)["application_name"]
            )
        )

        for serviceID in app["microservices"]:
            # wait until lock is released
            while server_address and lock:
                ic("Waiting for lock...")
                time.sleep(10)
                continue

            lock = True
            deploy(serviceID)

            last_service = serviceID

    ic("Finished app stress testing")


def add_addon_marketplace(addon):
    ic("Registering addon...")
    url = f"http://{ADDONS_MARKETPLACE_URL}/api/v1/marketplace/addons"
    resp = requests.post(url, json=addon)
    if resp.status_code >= 400:
        ic("Addon registration failed")
        ic(resp)
        exit(1)

    resp = resp.json()
    ic(resp)

    return resp


def install_addon(id):
    ic("Installing addon...")
    url = f"http://{ADDONS_ENGINE_URL}/api/v1/addons"
    resp = requests.post(url, json={"marketplace_id": id})
    if resp.status_code >= 400:
        ic("Addon installation failed")
        ic(resp)
        exit(1)

    resp = resp.json()
    ic(resp)

    return resp


def delete_all_addons():
    ic("Deleting all addons...")
    url = f"http://{ADDONS_ENGINE_URL}/api/v1/addons/"
    resp = requests.delete(url)
    if resp.status_code >= 400:
        ic("Addon deletion failed")
        ic(resp)
        exit(1)

    url = f"http://{ADDONS_MARKETPLACE_URL}/api/v1/marketplace/addons/"
    resp = requests.delete(url)
    if resp.status_code >= 400:
        ic("Addon deletion failed")
        ic(resp)
        exit(1)


def stress_addons_test(addons_count=10):
    for i in range(addons_count):
        addon = get_random_addon()
        registered_addon = add_addon_marketplace(addon)
        ic("sleep until addon is verified")
        time.sleep(20)

        id = registered_addon["_id"]
        install_addon(id)

        ic("sleep until addon is installed")
        time.sleep(10)

    ic("Finished addons stress testing")


def cleanup():
    try:
        cleanup_apps()
    except Exception:
        ic("Failed to cleanup apps")

    try:
        cleanup_addons()
    except Exception:
        ic("Failed to cleanup addons")


def cleanup_apps():
    ic("Cleaning up...Deleting all apps...")
    delete_all_apps()
    ic("Done deleting apps...")


def cleanup_addons():
    ic("Cleaning up...Deleting all addons...")
    delete_all_addons()
    ic("Done deleting addons...")


def print_csv():
    global results

    with open("deployment_time.csv", "w+") as my_csv:
        csvWriter = csv.writer(my_csv, delimiter=",")
        csvWriter.writerows(results)


@app.route("/")
def index():
    global start, stop, lock, last_service
    global results, service_count

    stop = time.time()
    if not start:
        ic("start was not initialized.")
        start = stop

    ic("Serive deployed.")
    service_time = stop - start
    service_count += 1
    results.append([service_count, service_time])
    ic(f"Time taken for {last_service}: {service_time}")

    # reset static variables
    start = None
    stop = None
    lock = False

    return "", 200


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Stress test script.")
    parser.add_argument(
        "--apps", type=int, help="Run stress test for apps (number of apps)", default=0
    )
    parser.add_argument(
        "--addons", type=int, help="Run stress test for addons (number of addons)", default=0
    )
    parser.add_argument("-a", "--address", type=str, help="Server address", default=None)

    args = parser.parse_args()
    address = args.address
    apps_count = args.apps
    addons_count = args.addons

    cleanup()

    app_thread = threading.Thread(
        target=stress_app_test,
        args=(
            apps_count,
            address,
        ),
        daemon=True,
    )
    addon_thread = threading.Thread(
        target=stress_addons_test,
        args=(addons_count,),
        daemon=True,
    )

    if apps_count:
        ic("Starting stress test for apps...")
        app_thread.start()

    if addons_count:
        ic("Starting stress test for addons...")
        addon_thread.start()

    address_thread = threading.Thread(
        target=app.run,
        kwargs={"host": "0.0.0.0", "port": 5001},
        daemon=True,
    )

    if address:
        address_thread.start()

    if app_thread.is_alive():
        app_thread.join()

    if addon_thread.is_alive():
        addon_thread.join()

    print_csv()

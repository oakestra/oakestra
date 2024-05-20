import logging
import os
import threading

import requests
from addons_runner.runner_types import get_runner
from db import addons_db

MARKETPLACE_API = f"{os.environ.get('MARKETPLACE_ADDR')}/api/v1/marketplace/addons"

manager_id = None


def stop_addon(addon, done=None):
    # when runner is not specified, we assume it's docker
    runner_type = addon.get("runner", "docker")
    runner_engine = get_runner(runner_type)(manager_id)

    runner_engine.stop_addon(addon)

    if done:
        done()


def get_addon_in_marketplace(marketplace_id):
    response = requests.get(f"{MARKETPLACE_API}/{marketplace_id}")
    response.raise_for_status()

    return response.json()


def run_addon(addon, done=None):
    runner_type = addon.get("runner", "docker")
    runner_engine = get_runner(runner_type)(manager_id)

    result = runner_engine.run_addon(addon)
    all_services = addon.get("services", [])
    failed_services = result.get("failed_services", [])

    new_status = "enabled"
    if failed_services:
        logging.error(f"Failed to run services: {failed_services}")
        if len(failed_services) == len(all_services):
            new_status = "failed"
        else:
            new_status = "partially_enabled"

    if done:
        done(new_status)


def run_active_addons():
    addons = addons_db.find_active_addons()

    for addon in addons:
        run_addon(
            addon,
            done=lambda new_status: addons_db.update_addon(
                addon.get("_id"), {"status": new_status}
            ),
        )


def install_addon(addon):
    marketplace_id = addon.get("marketplace_id")

    marketplace_addon = get_addon_in_marketplace(marketplace_id)
    services = marketplace_addon.get("services", [])

    if not services:
        logging.error(f"Addon-{marketplace_id} has no services")
        return None

    addon["services"] = services
    addon["status"] = "installing"
    created_addon = addons_db.create_addon(addon)
    addon_id = str(created_addon.get("_id"))

    def on_complete(new_status):
        addons_db.update_addon(addon_id, {"status": new_status})

    threading.Thread(
        target=run_addon,
        args=(created_addon,),
        kwargs={"done": on_complete},
    ).start()

    return created_addon


def stop_all_addons():
    addons = addons_db.find_active_addons()
    for addon in addons:
        stop_addon(addon)


def init_addon_manager(addon_manager_id, start_active_addons=False):
    global manager_id

    manager_id = addon_manager_id
    if start_active_addons:
        threading.Thread(target=run_active_addons, daemon=True).start()

import logging
import os
import uuid

import requests
from flask import Flask, request

MY_PORT = os.environ.get("MY_PORT") or 11111
RESOURCE_ABSTRACTOR_ADDR = os.environ.get("RESOURCE_ABSTRACTOR_ADDR") or "http://0.0.0.0:11011"
HOOKS_API = f"{RESOURCE_ABSTRACTOR_ADDR}/api/v1/hooks"
SERVICE_NAME = os.environ.get("SERVICE_NAME") or "0.0.0.0"
SYNC_OPERATION = os.environ.get("SYNC_OPERATION") or False

logging.basicConfig(level=logging.INFO)

app = Flask(__name__)

app.config["OPENAPI_VERSION"] = "3.0.2"
app.config["API_TITLE"] = "Stupid Server"
app.config["API_VERSION"] = "v1"
app.config["OPENAPI_URL_PREFIX"] = "/docs"

id = str(uuid.uuid4())


@app.route("/stupid-server", methods=["POST"])
def dummy_before():
    body = request.get_json()
    body[id] = id
    logging.info(f"{id}-body: {body}")

    return body


@app.route("/stupid-server/async", methods=["POST"])
def dummy_after():
    body = request.get_json()
    logging.info(f"{id}-body: {body}")

    return body


@app.route("/", methods=["GET"])
def health():
    return "ok"


@app.route("/register", methods=["GET"])
def register_endpoint():
    return register_hook()


def register_hook():
    webhook_url = f"http://{SERVICE_NAME}:{MY_PORT}/stupid-server"
    events = ["beforeCreate" if bool(SYNC_OPERATION) else "afterCreate"]

    if not SYNC_OPERATION:
        webhook_url += "/async"

    hook = {
        "hook_name": f"{SERVICE_NAME}:{MY_PORT}",
        "webhook_url": webhook_url,
        "entity": "resources",
        "events": events,
    }

    try:
        response = requests.post(HOOKS_API, json=hook)
        response.raise_for_status()
        logging.info(response.json())
    except Exception as e:
        logging.error(f"Failed with error: {e}")


if __name__ == "__main__":
    register_hook()
    app.run(host="::", port=int(MY_PORT), debug=False)

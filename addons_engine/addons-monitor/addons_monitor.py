import logging
import os
import time

import socketio
from services import monitor_service

ADDONS_MANAGER_ADDR = os.environ.get("ADDONS_MANAGER_ADDR") or "http://localhost:11101"
WAIT_TIME = os.environ.get("WAIT_TIME") or 30  # seconds

# Syncronous socketio client
socketio = socketio.Client()

if __name__ == "__main__":
    addon_manager = None

    def on_receive_manager_id(manager_id):
        global addon_manager

        logging.info(f"Received manager id: {manager_id}")
        addon_manager = monitor_service.init_addons_monitor(manager_id, socketio)

    def on_connect():
        logging.info("Successfully connected to addon manager!")
        socketio.emit("get_manager_id")

    socketio.on("connect", on_connect)
    socketio.on("receive_manager_id", on_receive_manager_id)

    # will keep trying to connect for 30 seconds
    try:
        socketio.connect(ADDONS_MANAGER_ADDR, wait_timeout=WAIT_TIME)
    except Exception as e:
        logging.error(f"Connecting to addons_manager failed: {e}")
        exit(1)

    # wait until addon_manager is initialized
    # TODO should we exit after some timeout?
    while not addon_manager:
        logging.info("Waiting for manager id...")
        time.sleep(int(WAIT_TIME))

    try:
        addon_manager.start_monitoring()  # This is a blocking call
    except Exception as e:
        logging.error(f"An error occurred while monitoring addons: {e}")

    socketio.disconnect()

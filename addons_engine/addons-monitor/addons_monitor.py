import logging
import os
import time

import socketio
from services import monitor_service

ADDONS_MANAGER_ADDR = os.environ.get("ADDONS_MANAGER_ADDR") or "http://localhost:11101"
WAIT_TIME = os.environ.get("WAIT_TIME") or 30  # seconds
MAX_RETRY = os.environ.get("MAX_RETRY") or 3

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

    max_retry = int(MAX_RETRY)
    while max_retry > 0:
        try:
            socketio.connect(ADDONS_MANAGER_ADDR)
            break
        except Exception as e:
            time.sleep(int(WAIT_TIME))
            max_retry -= 1

            if max_retry < 0:
                logging.error(f"Failed to connect to addons_manager: {e}")
                exit(1)

    # wait until addon_manager is initialized
    max_retry = int(MAX_RETRY)
    logging.info("Waiting for manager id...")
    while not addon_manager:
        time.sleep(int(WAIT_TIME))
        max_retry -= 1
        if max_retry < 0:
            logging.error("Failed to get manager id")
            exit(1)

    try:
        addon_manager.start_monitoring()  # This is a blocking call
    except Exception as e:
        logging.error(f"An error occurred while monitoring addons: {e}")

    socketio.disconnect()

import logging
import signal
import sys

from services.monitor_service import addons_monitor
from utils.cleanup_handler import handle_shutdown

# TODO remove this
logging.basicConfig(level=logging.INFO)


def signal_handler(sig, frame):
    logging.info("Shutting down Addon Manager...")
    handle_shutdown()

    sys.exit(0)



logging.basicConfig(level=logging.INFO)

if __name__ == "__main__":
    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)
    try:
        addons_monitor.start_monitoring()  # This is a blocking call
    except Exception as e:
        logging.error(f"An error occurred while monitoring addons: {e}")

import logging
import os
import signal
import sys

from services.monitor_service import addons_monitor
from utils.cleanup_handler import handle_shutdown


def signal_handler(sig, frame):
    logging.info("Shutting down Addon Monitor...")
    handle_shutdown()

    sys.exit(0)


def configure_logging():
    debug = os.environ.get("FLASK_DEBUG", "False")

    log_level = logging.WARNING
    if debug != "False":
        log_level = logging.DEBUG

    logging.basicConfig(
        level=log_level,
        format="%(asctime)s - %(levelname)s - %(message)s",
        handlers=[logging.StreamHandler(sys.stdout)],
    )


if __name__ == "__main__":
    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)
    configure_logging()
    try:
        addons_monitor.start_monitoring()  # This is a blocking call
    except Exception as e:
        logging.error("An error occurred while monitoring addons", exc_info=e)

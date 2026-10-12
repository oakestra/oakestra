import os
import signal
import sys

from oakestra_logging import configure_logging, get_logger
from services.monitor_service import addons_monitor
from utils.cleanup_handler import handle_shutdown

configure_logging(os.getenv("OAKESTRA_SERVICE_NAME", "addons_monitor"))
logger = get_logger(__name__)


def signal_handler(sig, frame):
    logger.info("Shutting down Addon Monitor", event_name="addons.monitor.stopping")
    handle_shutdown()

    sys.exit(0)


if __name__ == "__main__":
    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)
    try:
        addons_monitor.start_monitoring()  # This is a blocking call
    except Exception:
        logger.critical(
            "Addon Monitor stopped after an unrecoverable failure",
            event_name="addons.monitor.unrecoverable",
            exc_info=True,
        )

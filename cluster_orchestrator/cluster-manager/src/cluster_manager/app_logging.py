import logging
import sys
from logging.handlers import RotatingFileHandler

from .app_config import CONFIG


# TODO: I think there's an issue having both the custom and root logger write to the same file
def configure_logging() -> logging.Logger:
    # Get log level from environment variable, default to DEBUG
    log_level_str = CONFIG.log_level.upper() if CONFIG.log_level else "DEBUG"
    log_level = getattr(logging, log_level_str, logging.DEBUG)

    format_str = "%(asctime)s - %(name)s - %(levelname)s - %(message)s"
    my_filename = "sm.log"

    logging.basicConfig(filename=my_filename, format=format_str, level=log_level)

    my_logger = logging.getLogger("cluster_manager")
    my_logger.setLevel(log_level)

    stdout_handler = logging.StreamHandler(sys.stdout)
    stdout_handler.setLevel(log_level)
    stdout_handler.setFormatter(logging.Formatter(format_str))
    my_logger.addHandler(stdout_handler)

    rotating_handler = RotatingFileHandler(my_filename, maxBytes=1500, backupCount=2)
    rotating_handler.setLevel(log_level)
    my_logger.addHandler(rotating_handler)

    return my_logger

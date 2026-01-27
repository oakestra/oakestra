import logging
import os
import sys
from logging.handlers import RotatingFileHandler


def configure_logging():
    # Get log level from environment variable, default to DEBUG
    log_level_str = os.environ.get("LOG_LEVEL", "DEBUG").upper()
    log_level = getattr(logging, log_level_str, logging.DEBUG)
    
    format_str = "%(asctime)s - %(name)s - %(levelname)s - %(message)s"
    formatter = logging.Formatter(format_str)
    my_filename = "sm.log"

    logging.basicConfig(filename=my_filename, format=format_str, level=log_level)
    my_logger = logging.getLogger("system_manager")
    my_logger.setLevel(log_level)

    stdout_handler = logging.StreamHandler(sys.stdout)
    stdout_handler.setLevel(log_level)
    formatter = logging.Formatter("%(asctime)s - %(name)s - %(levelname)s - %(message)s")
    stdout_handler.setFormatter(formatter)
    my_logger.addHandler(stdout_handler)

    rotating_handler = RotatingFileHandler(my_filename, maxBytes=1500, backupCount=2)
    rotating_handler.setLevel(log_level)
    my_logger.addHandler(rotating_handler)

    return my_logger

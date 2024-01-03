import logging
import sys
from logging.handlers import RotatingFileHandler


def configure_logging():
    format_str = "%(asctime)s - - - %(name)s - %(levelname)s - %(message)s"
    formatter = logging.Formatter(format_str)
    my_filename = "cs.log"

    logging.basicConfig(filename=my_filename, format=format_str, level=logging.DEBUG)
    my_logger = logging.getLogger("")

    stdout_handler = logging.StreamHandler(sys.stdout)
    stdout_handler.setLevel(logging.DEBUG)
    formatter = logging.Formatter("%(asctime)s  - - - %(name)s - %(levelname)s - %(message)s")
    stdout_handler.setFormatter(formatter)
    my_logger.addHandler(stdout_handler)

    rotating_handler = RotatingFileHandler(my_filename, maxBytes=1500, backupCount=2)
    my_logger.addHandler(rotating_handler)

    return my_logger

import os
import sys
import logging

from logging.handlers import RotatingFileHandler


def configure_logging(name):
    format_str = '%(asctime)s - %(name)s - %(levelname)s - %(message)s'
    formatter = logging.Formatter(format_str)
    my_filename = 'app/log/ne.log'

    logging.basicConfig(filename=my_filename, format=format_str, level=logging.DEBUG)
    my_logger = logging.getLogger(name)
    # Deactivate the default flask logger so that log messages don't get duplicated
    # from flask.logging import default_handler
    # my_logger.removeHandler(default_handler)
    stdout_handler = logging.StreamHandler(sys.stdout)
    stdout_handler.setLevel(logging.DEBUG)
    stdout_handler.setFormatter(formatter)
    my_logger.addHandler(stdout_handler)
    rotating_handler = RotatingFileHandler(my_filename, maxBytes=1500, backupCount=2)
    my_logger.addHandler(rotating_handler)
    return my_logger

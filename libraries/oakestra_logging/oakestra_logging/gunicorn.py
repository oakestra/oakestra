"""Gunicorn integration for Oakestra's shared JSON logging contract."""

from __future__ import annotations

import os

from gunicorn.glogging import Logger

from .config import configure_gunicorn_loggers, configure_logging


class StructuredGunicornLogger(Logger):
    """Send Gunicorn-owned records through Oakestra's stdout JSON handler."""

    def setup(self, cfg):
        super().setup(cfg)
        configure_logging(os.getenv("OAKESTRA_SERVICE_NAME", "gunicorn"))
        configure_gunicorn_loggers()

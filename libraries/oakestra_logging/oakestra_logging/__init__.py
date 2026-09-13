"""Shared structured logging for Oakestra Python services."""

from .config import configure_logging, get_logger

__all__ = ["configure_logging", "get_logger"]
__version__ = "0.1.0"

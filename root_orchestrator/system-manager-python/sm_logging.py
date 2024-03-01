import json
import logging
from logging.handlers import RotatingFileHandler
import datetime


class JsonFormatter(logging.Formatter):
    def __init__(self, service_name="system_manager"):
        super().__init__()
        self.service_name = service_name

    def format(self, record):
        log_time = datetime.datetime.fromtimestamp(record.created).isoformat()
        contextual_param = {key: value for key, value in record.__dict__.get("context", {}).items()}

        data = {
            "level": record.levelname,
            "timestamp": log_time,
            "service": self.service_name,
            "filename": record.filename,
            "line_no": record.lineno,
            "message": super().format(record),
            "context": contextual_param,
        }

        return json.dumps(data)


class LogfmtFormatter(logging.Formatter):
    def __init__(self, service_name="system_manager"):
        super().__init__()
        self.service_name = service_name

    def format(self, record):
        unix_timestamp_ms = int(
            datetime.datetime.fromtimestamp(record.created).timestamp() * 1000
        )
        log_time = f"{unix_timestamp_ms:.6f}"
        contextual_param = ""
        extra_info = record.__dict__.get("context", {}).items()
        message = super().format(record)
        for key, value in extra_info:
            contextual_param += f"{key}={value}"
        return (
            f"level={record.levelname} ts={log_time} service={self.service_name} "
            f"file={record.filename} file_no={record.lineno}"
            f"message={message} {contextual_param}"
        )


class CustomFormatter(logging.Formatter):
    def __init__(self, service_name="system_manager"):
        super().__init__()
        self.service_name = service_name

    def format(self, record):
        log_time = datetime.datetime.fromtimestamp(record.created).strftime("%Y-%m-%d %H:%M:%S")
        log_msg = super().format(record)
        contextual_param = ""
        extra_info = record.__dict__.get("context", {}).items()
        for key, value in extra_info:
            contextual_param += f"{key}={value} "

        return (
            f"[{record.levelname[0]}{log_time} {self.service_name} "
            f"{record.filename}:{record.lineno}] {log_msg} {contextual_param}"
        )


class CustomLogger:
    def __init__(self, config={}):
        format = config.get("format", "default")
        filename = config.get("filename", "sm.log")

        self.logger = logging.getLogger(__name__)
        self.logger.setLevel(logging.DEBUG)

        self.add_logging_methods()

        formatter = self.get_formatter(format)
        ch = logging.StreamHandler()
        ch.setFormatter(formatter)
        self.logger.addHandler(ch)

        rotating_handler = RotatingFileHandler(filename, maxBytes=1500, backupCount=2)
        self.logger.addHandler(rotating_handler)

    def get_formatter(self, format):

        formatter_mapping = {
            "default": CustomFormatter(),
            "json": JsonFormatter(),
            "logfmt": LogfmtFormatter(),
        }
        return formatter_mapping.get(format, CustomFormatter())

    def add_logging_methods(self):
        logging_levels = ["debug", "info", "warning", "error", "critical"]
        for level in logging_levels:
            method = getattr(self.logger, level)
            setattr(self, level, method)


def configure_logging():
    """Configures logging for the system manager.

    Returns the configured logger instance.
    """
    log_config = {
        "format": "default",
        "filename": "sm.log",
    }
    global logger
    logger = CustomLogger(log_config)
    logger.info("Logging configured with parameters: " + json.dumps(log_config))
    return logger

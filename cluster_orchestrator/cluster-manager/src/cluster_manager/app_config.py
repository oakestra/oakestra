import os
from dataclasses import dataclass


def _load_optional_env_str(name: str) -> str | None:
    return os.environ.get(name)


def _load_required_env_str(name: str) -> str:
    value = os.environ.get(name)
    if not value:
        raise RuntimeError(f"Required environment variable '${name}' is not set.")
    return value


def _load_optional_env_int(name: str) -> int | None:
    value = os.environ.get(name)
    if not value:
        return None

    try:
        return int(value)
    except ValueError:
        raise RuntimeError(f"Environment variable '${name}' is not an integer.")


def _load_required_env_int(name: str) -> int:
    value = _load_optional_env_int(name)
    if not value:
        raise RuntimeError(f"Required environment variable '${name}' is not set.")
    return value


def _build_http_url(host: str, port: int) -> str:
    return "http://" + host + ":" + str(port)


@dataclass(frozen=True)
class EnvironmentConfig:
    # required vars
    port: int
    mqtt_broker_url: str
    mqtt_broker_port: int
    cluster_scheduler_url: str
    cluster_scheduler_port: int
    cluster_service_manager_addr: str
    cluster_service_manager_port: int
    system_manager_url: str
    system_manager_port: int
    system_manager_grpc_port: int
    cluster_address: str

    # could be optional vars?
    cluster_name: str
    cluster_location: str

    # optional vars
    mqtt_cert: str | None
    cluster_keyfile_password: str | None
    log_level: str | None
    aggregation_interval: int | None  # default 15
    node_scheduled_timeout: int | None  # default 15

    @classmethod
    def load(cls) -> "EnvironmentConfig":
        return cls(
            port=_load_required_env_int("MY_PORT"),
            mqtt_broker_url=_load_required_env_str("MQTT_BROKER_URL"),
            mqtt_broker_port=_load_required_env_int("MQTT_BROKER_PORT"),
            cluster_scheduler_url=_load_required_env_str("CLUSTER_SCHEDULER_URL"),
            cluster_scheduler_port=_load_required_env_int("CLUSTER_SCHEDULER_PORT"),
            cluster_service_manager_addr=_load_required_env_str("CLUSTER_SERVICE_MANAGER_ADDR"),
            cluster_service_manager_port=_load_required_env_int("CLUSTER_SERVICE_MANAGER_PORT"),
            system_manager_url=_load_required_env_str("SYSTEM_MANAGER_URL"),
            system_manager_port=_load_required_env_int("SYSTEM_MANAGER_PORT"),
            system_manager_grpc_port=_load_required_env_int("SYSTEM_MANAGER_GRPC_PORT"),
            cluster_address=_load_required_env_str("CLUSTER_ADDRESS"),
            cluster_name=_load_required_env_str("CLUSTER_NAME"),
            cluster_location=_load_required_env_str("CLUSTER_LOCATION"),
            mqtt_cert=_load_optional_env_str("MQTT_CERT"),
            cluster_keyfile_password=_load_optional_env_str("CLUSTER_KEYFILE_PASSWORD"),
            log_level=_load_optional_env_str("LOG_LEVEL"),
            aggregation_interval=_load_optional_env_int("AGGREGATION_INTERVAL"),
            node_scheduled_timeout=_load_optional_env_int("NODE_SCHEDULED_TIMEOUT"),
        )


CONFIG = EnvironmentConfig.load()
GRPC_REQUEST_TIMEOUT = 120

SCHEDULER_ADDR = _build_http_url(CONFIG.cluster_scheduler_url, CONFIG.cluster_scheduler_port)
SERVICE_MANAGER_ADDR = _build_http_url(
    CONFIG.cluster_service_manager_addr, CONFIG.cluster_service_manager_port
)
SYSTEM_MANAGER_GRPC_ADDR = CONFIG.system_manager_url + ":" + str(CONFIG.system_manager_grpc_port)
SYSTEM_MANAGER_ADDR = _build_http_url(CONFIG.system_manager_url, CONFIG.system_manager_port)


@dataclass
class RuntimeConfig:
    assigned_cluster_id: str | None = None


RUNTIME_CONFIG = RuntimeConfig()

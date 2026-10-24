from dataclasses import dataclass
from typing import Any

from pydantic import BaseModel, ConfigDict, field_validator


class WorkerCsiDriver(BaseModel):
    model_config = ConfigDict(extra="allow")

    csi_driver_name: str | None = None

    @field_validator("csi_driver_name", mode="before")
    @classmethod
    def empty_string_to_none(cls, value: Any) -> Any:
        if value == "":
            return None
        return value


class WorkerMetrics(BaseModel):
    model_config = ConfigDict(extra="allow")

    # TODO: I don't think resource-abstractor returns this
    architecture: str | None = None

    vcpus: int | None = None
    memory: int | None = None
    vgpus: int | None = None
    vram: float | None = None

    cpu_percent: float | None = None
    memory_percent: float | None = None
    vram_percent: float | None = None
    gpu_temp: float | None = None

    # TODO: I don't think resource-abstractor returns this (only 'gpu_percent')
    gpu_usage: float | None = None

    # TODO: I don't think resource-abstractor returns this (only 'gpu_drivers')
    gpu_driver: str | None = None

    virtualization: list[str] = []
    supported_addons: list[str] = []
    csi_drivers: list[WorkerCsiDriver] = []

    @field_validator("virtualization", "supported_addons", "csi_drivers", mode="before")
    @classmethod
    def none_to_empty_list(cls, value: Any) -> Any:
        if value is None:
            return []
        return value

    @field_validator("architecture", "gpu_driver", mode="before")
    @classmethod
    def empty_string_to_none(cls, value: Any) -> Any:
        if value == "":
            return None
        return value


@dataclass
class AggregatedWorkerMetrics:
    active_nodes: int
    vcpus: int
    memory: int
    vgpus: int
    vram: int

    cpu_percent: float
    memory_percent: float
    vram_percent: float
    gpu_percent: float

    gpu_drivers: list[str]

    virtualization: list[str]
    supported_addons: list[str]
    csi_drivers: list[Any]

    aggregation_per_architecture: dict[str, "AggregatedWorkerMetrics"]

from typing import Any

from pydantic import BaseModel, ConfigDict, Field, field_validator


class JobInstance(BaseModel):
    model_config = ConfigDict(extra="allow")

    instance_number: int | None = None
    status: str | None = None
    status_detail: str | None = None
    public_ip: str | None = Field(alias="publicip", default=None)

    # timestamp in epoch seconds
    last_modified_timestamp: float | None = None

    cpu_percent: str | None = None
    memory_percent: str | None = None
    disk: str | None = None
    logs: str | None = None

    host_ip: str | None = None
    host_port: int | None = None
    worker_id: str | None = None

    @field_validator(
        "status",
        "status_detail",
        "public_ip",
        "cpu_percent",
        "memory_percent",
        "disk",
        "logs",
        "host_ip",
        "worker_id",
        mode="before",
    )
    @classmethod
    def empty_string_to_none(cls, value: Any) -> Any:
        if value == "":
            return None
        return value

    def require_instance_number(self) -> int:
        if self.instance_number is None:
            raise RuntimeError("Expected instance to have instance_number")
        return self.instance_number

    def update(self, other: "JobInstance") -> None:
        if other.status is not None:
            self.status = other.status
        if other.status_detail is not None:
            self.status = other.status_detail
        if other.last_modified_timestamp is not None:
            self.last_modified_timestamp = other.last_modified_timestamp
        if other.public_ip is not None:
            self.public_ip = other.public_ip
        if other.cpu_percent is not None:
            self.cpu_percent = other.cpu_percent
        if other.memory_percent is not None:
            self.memory_percent = other.memory_percent
        if other.disk is not None:
            self.disk = other.disk
        if other.logs is not None:
            self.logs = other.logs


class Job(BaseModel):
    model_config = ConfigDict(extra="allow")

    id: str | None = Field(alias="_id", default=None)
    job_name: str | None = None
    status: str | None = None
    status_detail: str | None = None
    instance_list: list[JobInstance] | None = None
    virtualization: str | None = None

    @field_validator("id", mode="before")
    @classmethod
    def transform_id(cls, value: Any) -> str | None:
        if value is None:
            return value

        if isinstance(value, str):
            return value if value != "" else None

        if isinstance(value, (int, float)):
            return str(value)

        raise ValueError("Unexpected type")

    @field_validator("job_name", "status", "status_detail", "virtualization", mode="before")
    @classmethod
    def empty_string_to_none(cls, value: Any) -> Any:
        if value == "":
            return None
        return value

    def require_id(self) -> str:
        if self.id is None:
            raise RuntimeError("Expected Job to have _id")
        return self.id

    def require_job_name(self) -> str:
        if self.job_name is None:
            raise RuntimeError("Expected Job to have job_name")
        return self.job_name


class JobInstanceResources(BaseModel):
    model_config = ConfigDict(extra="allow")

    job_name: str | None = None
    instance_number: int | None = Field(default=None, alias="instance")
    virtualization: str | None = None
    logs: str | None = None
    status: str | None = None

    # These are explicitly defined as string in Go, but contain numbers or ""
    cpu_percent: str | None = None
    memory_percent: str | None = None
    disk: str | None = None

    @field_validator(
        "job_name",
        "virtualization",
        "logs",
        "status",
        "cpu_percent",
        "memory_percent",
        "disk",
        mode="before",
    )
    @classmethod
    def empty_string_to_none(cls, value: Any) -> Any:
        if value == "":
            return None
        return value

    def require_job_name(self) -> str:
        if self.job_name is None:
            raise RuntimeError("Expected instance resources to have job_name")
        return self.job_name

    def require_instance_number(self) -> int:
        if self.instance_number is None:
            raise RuntimeError("Expected instance resources to have instance")
        return self.instance_number

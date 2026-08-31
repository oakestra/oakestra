from typing import Any

from pydantic import BaseModel, ConfigDict, Field, field_validator

from .job import JobInstanceResources


class NodeInformationMessage(BaseModel):
    model_config = ConfigDict(extra="allow")


class NodeJobMessage(BaseModel):
    model_config = ConfigDict(extra="allow")

    job_name: str | None = Field(alias="sname", default=None)
    status: str | None = None
    status_detail: str | None = None
    instance_number: int = Field(alias="instance")
    public_ip: str | None = Field(alias="publicip", default=None)

    @field_validator("job_name", "status", "status_detail", "public_ip", mode="before")
    @classmethod
    def empty_string_to_none(cls, value: Any) -> Any:
        if value == "":
            return None
        return value


class NodeJobResourceMessage(BaseModel):
    model_config = ConfigDict(extra="allow")

    instance_resources: list[JobInstanceResources] = Field(alias="services", default=[])

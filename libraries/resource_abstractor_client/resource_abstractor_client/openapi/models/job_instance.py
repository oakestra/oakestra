from __future__ import annotations

from collections.abc import Mapping
from typing import TYPE_CHECKING, Any, TypeVar

from attrs import define as _attrs_define
from attrs import field as _attrs_field

from ..types import UNSET, Unset

if TYPE_CHECKING:
    from ..models.job_history_sample import JobHistorySample


T = TypeVar("T", bound="JobInstance")


@_attrs_define
class JobInstance:
    """One running copy of a job on one worker.

    Attributes:
        instance_number (int | Unset):
        worker_id (str | Unset):
        status (str | Unset):
        status_detail (str | Unset):
        host_ip (str | Unset):
        host_port (int | Unset):
        publicip (str | Unset):
        cpu_percent (float | Unset):
        memory_percent (float | Unset):
        disk (float | Unset):
        logs (str | Unset):
        last_modified_timestamp (float | Unset): Unix seconds. Unlike cpu_history/memory_history's timestamp
            (server-generated on every PATCH), this one is taken from the
            request body verbatim - the caller (cluster_manager) sets it
            before sending.
        cpu_history (list[JobHistorySample] | Unset):
        memory_history (list[JobHistorySample] | Unset):
    """

    instance_number: int | Unset = UNSET
    worker_id: str | Unset = UNSET
    status: str | Unset = UNSET
    status_detail: str | Unset = UNSET
    host_ip: str | Unset = UNSET
    host_port: int | Unset = UNSET
    publicip: str | Unset = UNSET
    cpu_percent: float | Unset = UNSET
    memory_percent: float | Unset = UNSET
    disk: float | Unset = UNSET
    logs: str | Unset = UNSET
    last_modified_timestamp: float | Unset = UNSET
    cpu_history: list[JobHistorySample] | Unset = UNSET
    memory_history: list[JobHistorySample] | Unset = UNSET
    additional_properties: dict[str, Any] = _attrs_field(init=False, factory=dict)

    def to_dict(self) -> dict[str, Any]:
        instance_number = self.instance_number

        worker_id = self.worker_id

        status = self.status

        status_detail = self.status_detail

        host_ip = self.host_ip

        host_port = self.host_port

        publicip = self.publicip

        cpu_percent = self.cpu_percent

        memory_percent = self.memory_percent

        disk = self.disk

        logs = self.logs

        last_modified_timestamp = self.last_modified_timestamp

        cpu_history: list[dict[str, Any]] | Unset = UNSET
        if not isinstance(self.cpu_history, Unset):
            cpu_history = []
            for cpu_history_item_data in self.cpu_history:
                cpu_history_item = cpu_history_item_data.to_dict()
                cpu_history.append(cpu_history_item)

        memory_history: list[dict[str, Any]] | Unset = UNSET
        if not isinstance(self.memory_history, Unset):
            memory_history = []
            for memory_history_item_data in self.memory_history:
                memory_history_item = memory_history_item_data.to_dict()
                memory_history.append(memory_history_item)

        field_dict: dict[str, Any] = {}
        field_dict.update(self.additional_properties)
        field_dict.update({})
        if instance_number is not UNSET:
            field_dict["instance_number"] = instance_number
        if worker_id is not UNSET:
            field_dict["worker_id"] = worker_id
        if status is not UNSET:
            field_dict["status"] = status
        if status_detail is not UNSET:
            field_dict["status_detail"] = status_detail
        if host_ip is not UNSET:
            field_dict["host_ip"] = host_ip
        if host_port is not UNSET:
            field_dict["host_port"] = host_port
        if publicip is not UNSET:
            field_dict["publicip"] = publicip
        if cpu_percent is not UNSET:
            field_dict["cpu_percent"] = cpu_percent
        if memory_percent is not UNSET:
            field_dict["memory_percent"] = memory_percent
        if disk is not UNSET:
            field_dict["disk"] = disk
        if logs is not UNSET:
            field_dict["logs"] = logs
        if last_modified_timestamp is not UNSET:
            field_dict["last_modified_timestamp"] = last_modified_timestamp
        if cpu_history is not UNSET:
            field_dict["cpu_history"] = cpu_history
        if memory_history is not UNSET:
            field_dict["memory_history"] = memory_history

        return field_dict

    @classmethod
    def from_dict(cls: type[T], src_dict: Mapping[str, Any]) -> T:
        from ..models.job_history_sample import JobHistorySample

        d = dict(src_dict)
        instance_number = d.pop("instance_number", UNSET)

        worker_id = d.pop("worker_id", UNSET)

        status = d.pop("status", UNSET)

        status_detail = d.pop("status_detail", UNSET)

        host_ip = d.pop("host_ip", UNSET)

        host_port = d.pop("host_port", UNSET)

        publicip = d.pop("publicip", UNSET)

        cpu_percent = d.pop("cpu_percent", UNSET)

        memory_percent = d.pop("memory_percent", UNSET)

        disk = d.pop("disk", UNSET)

        logs = d.pop("logs", UNSET)

        last_modified_timestamp = d.pop("last_modified_timestamp", UNSET)

        _cpu_history = d.pop("cpu_history", UNSET)
        cpu_history: list[JobHistorySample] | Unset = UNSET
        if _cpu_history is not UNSET:
            cpu_history = []
            for cpu_history_item_data in _cpu_history:
                cpu_history_item = JobHistorySample.from_dict(cpu_history_item_data)

                cpu_history.append(cpu_history_item)

        _memory_history = d.pop("memory_history", UNSET)
        memory_history: list[JobHistorySample] | Unset = UNSET
        if _memory_history is not UNSET:
            memory_history = []
            for memory_history_item_data in _memory_history:
                memory_history_item = JobHistorySample.from_dict(memory_history_item_data)

                memory_history.append(memory_history_item)

        job_instance = cls(
            instance_number=instance_number,
            worker_id=worker_id,
            status=status,
            status_detail=status_detail,
            host_ip=host_ip,
            host_port=host_port,
            publicip=publicip,
            cpu_percent=cpu_percent,
            memory_percent=memory_percent,
            disk=disk,
            logs=logs,
            last_modified_timestamp=last_modified_timestamp,
            cpu_history=cpu_history,
            memory_history=memory_history,
        )

        job_instance.additional_properties = d
        return job_instance

    @property
    def additional_keys(self) -> list[str]:
        return list(self.additional_properties.keys())

    def __getitem__(self, key: str) -> Any:
        return self.additional_properties[key]

    def __setitem__(self, key: str, value: Any) -> None:
        self.additional_properties[key] = value

    def __delitem__(self, key: str) -> None:
        del self.additional_properties[key]

    def __contains__(self, key: str) -> bool:
        return key in self.additional_properties

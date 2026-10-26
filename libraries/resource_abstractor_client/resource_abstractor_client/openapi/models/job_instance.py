from __future__ import annotations

from collections.abc import Mapping
from typing import TYPE_CHECKING, Any, TypeVar, cast

from attrs import define as _attrs_define
from attrs import field as _attrs_field

from ..types import UNSET, Unset

if TYPE_CHECKING:
    from ..models.job_history_sample import JobHistorySample


T = TypeVar("T", bound="JobInstance")


@_attrs_define
class JobInstance:
    """One running copy of a job on one worker.

    cpu_percent, memory_percent, disk and host_port also accept a numeric
    string on write, for compatibility with NodeEngine/cluster_manager
    (which send them that way) and documents written by the older Python
    service. The service always responds with a number.

        Attributes:
            instance_number (int | None | Unset):
            worker_id (None | str | Unset):
            status (None | str | Unset):
            status_detail (None | str | Unset):
            host_ip (None | str | Unset):
            host_port (int | None | Unset):
            publicip (None | str | Unset):
            cpu_percent (float | None | Unset):
            memory_percent (float | None | Unset):
            disk (float | None | Unset):
            logs (None | str | Unset):
            last_modified_timestamp (float | None | Unset): Unix seconds. Unlike cpu_history/memory_history's timestamp
                (server-generated on every PATCH), this one is taken from the
                request body verbatim - the caller (cluster_manager) sets it
                before sending. `PATCH /jobs/{id}/{instance_id}` writes a
                literal null for any of these fields the request body omits
                (see internal/store/jobs.go's UpdateJobInstance), so every field
                above is nullable in a response even though none of them are
                required on write.
            cpu_history (list[JobHistorySample] | Unset):
            memory_history (list[JobHistorySample] | Unset):
    """

    instance_number: int | None | Unset = UNSET
    worker_id: None | str | Unset = UNSET
    status: None | str | Unset = UNSET
    status_detail: None | str | Unset = UNSET
    host_ip: None | str | Unset = UNSET
    host_port: int | None | Unset = UNSET
    publicip: None | str | Unset = UNSET
    cpu_percent: float | None | Unset = UNSET
    memory_percent: float | None | Unset = UNSET
    disk: float | None | Unset = UNSET
    logs: None | str | Unset = UNSET
    last_modified_timestamp: float | None | Unset = UNSET
    cpu_history: list[JobHistorySample] | Unset = UNSET
    memory_history: list[JobHistorySample] | Unset = UNSET
    additional_properties: dict[str, Any] = _attrs_field(init=False, factory=dict)

    def to_dict(self) -> dict[str, Any]:
        instance_number: int | None | Unset
        if isinstance(self.instance_number, Unset):
            instance_number = UNSET
        else:
            instance_number = self.instance_number

        worker_id: None | str | Unset
        if isinstance(self.worker_id, Unset):
            worker_id = UNSET
        else:
            worker_id = self.worker_id

        status: None | str | Unset
        if isinstance(self.status, Unset):
            status = UNSET
        else:
            status = self.status

        status_detail: None | str | Unset
        if isinstance(self.status_detail, Unset):
            status_detail = UNSET
        else:
            status_detail = self.status_detail

        host_ip: None | str | Unset
        if isinstance(self.host_ip, Unset):
            host_ip = UNSET
        else:
            host_ip = self.host_ip

        host_port: int | None | Unset
        if isinstance(self.host_port, Unset):
            host_port = UNSET
        else:
            host_port = self.host_port

        publicip: None | str | Unset
        if isinstance(self.publicip, Unset):
            publicip = UNSET
        else:
            publicip = self.publicip

        cpu_percent: float | None | Unset
        if isinstance(self.cpu_percent, Unset):
            cpu_percent = UNSET
        else:
            cpu_percent = self.cpu_percent

        memory_percent: float | None | Unset
        if isinstance(self.memory_percent, Unset):
            memory_percent = UNSET
        else:
            memory_percent = self.memory_percent

        disk: float | None | Unset
        if isinstance(self.disk, Unset):
            disk = UNSET
        else:
            disk = self.disk

        logs: None | str | Unset
        if isinstance(self.logs, Unset):
            logs = UNSET
        else:
            logs = self.logs

        last_modified_timestamp: float | None | Unset
        if isinstance(self.last_modified_timestamp, Unset):
            last_modified_timestamp = UNSET
        else:
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

        def _parse_instance_number(data: object) -> int | None | Unset:
            if data is None:
                return data
            if isinstance(data, Unset):
                return data
            return cast(int | None | Unset, data)

        instance_number = _parse_instance_number(d.pop("instance_number", UNSET))

        def _parse_worker_id(data: object) -> None | str | Unset:
            if data is None:
                return data
            if isinstance(data, Unset):
                return data
            return cast(None | str | Unset, data)

        worker_id = _parse_worker_id(d.pop("worker_id", UNSET))

        def _parse_status(data: object) -> None | str | Unset:
            if data is None:
                return data
            if isinstance(data, Unset):
                return data
            return cast(None | str | Unset, data)

        status = _parse_status(d.pop("status", UNSET))

        def _parse_status_detail(data: object) -> None | str | Unset:
            if data is None:
                return data
            if isinstance(data, Unset):
                return data
            return cast(None | str | Unset, data)

        status_detail = _parse_status_detail(d.pop("status_detail", UNSET))

        def _parse_host_ip(data: object) -> None | str | Unset:
            if data is None:
                return data
            if isinstance(data, Unset):
                return data
            return cast(None | str | Unset, data)

        host_ip = _parse_host_ip(d.pop("host_ip", UNSET))

        def _parse_host_port(data: object) -> int | None | Unset:
            if data is None:
                return data
            if isinstance(data, Unset):
                return data
            return cast(int | None | Unset, data)

        host_port = _parse_host_port(d.pop("host_port", UNSET))

        def _parse_publicip(data: object) -> None | str | Unset:
            if data is None:
                return data
            if isinstance(data, Unset):
                return data
            return cast(None | str | Unset, data)

        publicip = _parse_publicip(d.pop("publicip", UNSET))

        def _parse_cpu_percent(data: object) -> float | None | Unset:
            if data is None:
                return data
            if isinstance(data, Unset):
                return data
            return cast(float | None | Unset, data)

        cpu_percent = _parse_cpu_percent(d.pop("cpu_percent", UNSET))

        def _parse_memory_percent(data: object) -> float | None | Unset:
            if data is None:
                return data
            if isinstance(data, Unset):
                return data
            return cast(float | None | Unset, data)

        memory_percent = _parse_memory_percent(d.pop("memory_percent", UNSET))

        def _parse_disk(data: object) -> float | None | Unset:
            if data is None:
                return data
            if isinstance(data, Unset):
                return data
            return cast(float | None | Unset, data)

        disk = _parse_disk(d.pop("disk", UNSET))

        def _parse_logs(data: object) -> None | str | Unset:
            if data is None:
                return data
            if isinstance(data, Unset):
                return data
            return cast(None | str | Unset, data)

        logs = _parse_logs(d.pop("logs", UNSET))

        def _parse_last_modified_timestamp(data: object) -> float | None | Unset:
            if data is None:
                return data
            if isinstance(data, Unset):
                return data
            return cast(float | None | Unset, data)

        last_modified_timestamp = _parse_last_modified_timestamp(
            d.pop("last_modified_timestamp", UNSET)
        )

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

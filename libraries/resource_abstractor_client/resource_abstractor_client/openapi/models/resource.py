from __future__ import annotations

from collections.abc import Mapping
from typing import TYPE_CHECKING, Any, TypeVar, cast

from attrs import define as _attrs_define
from attrs import field as _attrs_field

from ..types import UNSET, Unset

if TYPE_CHECKING:
    from ..models.history_sample import HistorySample
    from ..models.resource_aggregation_per_architecture import ResourceAggregationPerArchitecture


T = TypeVar("T", bound="Resource")


@_attrs_define
class Resource:
    """A candidate resource: a worker node at cluster level, a whole cluster
    at root level. Unknown fields are stored and returned verbatim.

        Attributes:
            field_id (str | Unset):  Example: 65f1c0d2a1b2c3d4e5f60718.
            candidate_name (str | Unset):
            candidate_location (str | Unset):
            ip (str | Unset):
            port (str | Unset):
            architecture (str | Unset):
            active (bool | Unset): True when `last_modified_timestamp` falls within the last 30
                seconds. Computed per request rather than persisted, and outside
                the canonical projection - so it is only present on
                `GET /api/v1/resources` and only when asked for explicitly, with
                `?resources=active`.
            active_nodes (int | Unset):
            memory (int | Unset):
            vcpus (int | Unset):
            vgpus (int | Unset):
            cpu_percent (float | Unset):
            memory_percent (float | Unset):
            gpu_percent (int | Unset):
            aggregation_per_architecture (ResourceAggregationPerArchitecture | Unset):
            virtualization (list[str] | None | Unset):
            supported_addons (list[str] | None | Unset):
            csi_drivers (list[Any] | None | Unset):
            cpu_history (list[HistorySample] | Unset): Most recent 100 cpu samples, appended server-side.
            memory_history (list[HistorySample] | Unset): Most recent 100 memory samples, appended server-side.
            last_modified_timestamp (float | Unset): Unix seconds, refreshed server-side on every usage report.
    """

    field_id: str | Unset = UNSET
    candidate_name: str | Unset = UNSET
    candidate_location: str | Unset = UNSET
    ip: str | Unset = UNSET
    port: str | Unset = UNSET
    architecture: str | Unset = UNSET
    active: bool | Unset = UNSET
    active_nodes: int | Unset = UNSET
    memory: int | Unset = UNSET
    vcpus: int | Unset = UNSET
    vgpus: int | Unset = UNSET
    cpu_percent: float | Unset = UNSET
    memory_percent: float | Unset = UNSET
    gpu_percent: int | Unset = UNSET
    aggregation_per_architecture: ResourceAggregationPerArchitecture | Unset = UNSET
    virtualization: list[str] | None | Unset = UNSET
    supported_addons: list[str] | None | Unset = UNSET
    csi_drivers: list[Any] | None | Unset = UNSET
    cpu_history: list[HistorySample] | Unset = UNSET
    memory_history: list[HistorySample] | Unset = UNSET
    last_modified_timestamp: float | Unset = UNSET
    additional_properties: dict[str, Any] = _attrs_field(init=False, factory=dict)

    def to_dict(self) -> dict[str, Any]:
        field_id = self.field_id

        candidate_name = self.candidate_name

        candidate_location = self.candidate_location

        ip = self.ip

        port = self.port

        architecture = self.architecture

        active = self.active

        active_nodes = self.active_nodes

        memory = self.memory

        vcpus = self.vcpus

        vgpus = self.vgpus

        cpu_percent = self.cpu_percent

        memory_percent = self.memory_percent

        gpu_percent = self.gpu_percent

        aggregation_per_architecture: dict[str, Any] | Unset = UNSET
        if not isinstance(self.aggregation_per_architecture, Unset):
            aggregation_per_architecture = self.aggregation_per_architecture.to_dict()

        virtualization: list[str] | None | Unset
        if isinstance(self.virtualization, Unset):
            virtualization = UNSET
        elif isinstance(self.virtualization, list):
            virtualization = self.virtualization

        else:
            virtualization = self.virtualization

        supported_addons: list[str] | None | Unset
        if isinstance(self.supported_addons, Unset):
            supported_addons = UNSET
        elif isinstance(self.supported_addons, list):
            supported_addons = self.supported_addons

        else:
            supported_addons = self.supported_addons

        csi_drivers: list[Any] | None | Unset
        if isinstance(self.csi_drivers, Unset):
            csi_drivers = UNSET
        elif isinstance(self.csi_drivers, list):
            csi_drivers = self.csi_drivers

        else:
            csi_drivers = self.csi_drivers

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

        last_modified_timestamp = self.last_modified_timestamp

        field_dict: dict[str, Any] = {}
        field_dict.update(self.additional_properties)
        field_dict.update({})
        if field_id is not UNSET:
            field_dict["_id"] = field_id
        if candidate_name is not UNSET:
            field_dict["candidate_name"] = candidate_name
        if candidate_location is not UNSET:
            field_dict["candidate_location"] = candidate_location
        if ip is not UNSET:
            field_dict["ip"] = ip
        if port is not UNSET:
            field_dict["port"] = port
        if architecture is not UNSET:
            field_dict["architecture"] = architecture
        if active is not UNSET:
            field_dict["active"] = active
        if active_nodes is not UNSET:
            field_dict["active_nodes"] = active_nodes
        if memory is not UNSET:
            field_dict["memory"] = memory
        if vcpus is not UNSET:
            field_dict["vcpus"] = vcpus
        if vgpus is not UNSET:
            field_dict["vgpus"] = vgpus
        if cpu_percent is not UNSET:
            field_dict["cpu_percent"] = cpu_percent
        if memory_percent is not UNSET:
            field_dict["memory_percent"] = memory_percent
        if gpu_percent is not UNSET:
            field_dict["gpu_percent"] = gpu_percent
        if aggregation_per_architecture is not UNSET:
            field_dict["aggregation_per_architecture"] = aggregation_per_architecture
        if virtualization is not UNSET:
            field_dict["virtualization"] = virtualization
        if supported_addons is not UNSET:
            field_dict["supported_addons"] = supported_addons
        if csi_drivers is not UNSET:
            field_dict["csi_drivers"] = csi_drivers
        if cpu_history is not UNSET:
            field_dict["cpu_history"] = cpu_history
        if memory_history is not UNSET:
            field_dict["memory_history"] = memory_history
        if last_modified_timestamp is not UNSET:
            field_dict["last_modified_timestamp"] = last_modified_timestamp

        return field_dict

    @classmethod
    def from_dict(cls: type[T], src_dict: Mapping[str, Any]) -> T:
        from ..models.history_sample import HistorySample
        from ..models.resource_aggregation_per_architecture import (
            ResourceAggregationPerArchitecture,
        )

        d = dict(src_dict)
        field_id = d.pop("_id", UNSET)

        candidate_name = d.pop("candidate_name", UNSET)

        candidate_location = d.pop("candidate_location", UNSET)

        ip = d.pop("ip", UNSET)

        port = d.pop("port", UNSET)

        architecture = d.pop("architecture", UNSET)

        active = d.pop("active", UNSET)

        active_nodes = d.pop("active_nodes", UNSET)

        memory = d.pop("memory", UNSET)

        vcpus = d.pop("vcpus", UNSET)

        vgpus = d.pop("vgpus", UNSET)

        cpu_percent = d.pop("cpu_percent", UNSET)

        memory_percent = d.pop("memory_percent", UNSET)

        gpu_percent = d.pop("gpu_percent", UNSET)

        _aggregation_per_architecture = d.pop("aggregation_per_architecture", UNSET)
        aggregation_per_architecture: ResourceAggregationPerArchitecture | Unset
        if isinstance(_aggregation_per_architecture, Unset):
            aggregation_per_architecture = UNSET
        else:
            aggregation_per_architecture = ResourceAggregationPerArchitecture.from_dict(
                _aggregation_per_architecture
            )

        def _parse_virtualization(data: object) -> list[str] | None | Unset:
            if data is None:
                return data
            if isinstance(data, Unset):
                return data
            try:
                if not isinstance(data, list):
                    raise TypeError()
                virtualization_type_0 = cast(list[str], data)

                return virtualization_type_0
            except (TypeError, ValueError, AttributeError, KeyError):
                pass
            return cast(list[str] | None | Unset, data)

        virtualization = _parse_virtualization(d.pop("virtualization", UNSET))

        def _parse_supported_addons(data: object) -> list[str] | None | Unset:
            if data is None:
                return data
            if isinstance(data, Unset):
                return data
            try:
                if not isinstance(data, list):
                    raise TypeError()
                supported_addons_type_0 = cast(list[str], data)

                return supported_addons_type_0
            except (TypeError, ValueError, AttributeError, KeyError):
                pass
            return cast(list[str] | None | Unset, data)

        supported_addons = _parse_supported_addons(d.pop("supported_addons", UNSET))

        def _parse_csi_drivers(data: object) -> list[Any] | None | Unset:
            if data is None:
                return data
            if isinstance(data, Unset):
                return data
            try:
                if not isinstance(data, list):
                    raise TypeError()
                csi_drivers_type_0 = cast(list[Any], data)

                return csi_drivers_type_0
            except (TypeError, ValueError, AttributeError, KeyError):
                pass
            return cast(list[Any] | None | Unset, data)

        csi_drivers = _parse_csi_drivers(d.pop("csi_drivers", UNSET))

        _cpu_history = d.pop("cpu_history", UNSET)
        cpu_history: list[HistorySample] | Unset = UNSET
        if _cpu_history is not UNSET:
            cpu_history = []
            for cpu_history_item_data in _cpu_history:
                cpu_history_item = HistorySample.from_dict(cpu_history_item_data)

                cpu_history.append(cpu_history_item)

        _memory_history = d.pop("memory_history", UNSET)
        memory_history: list[HistorySample] | Unset = UNSET
        if _memory_history is not UNSET:
            memory_history = []
            for memory_history_item_data in _memory_history:
                memory_history_item = HistorySample.from_dict(memory_history_item_data)

                memory_history.append(memory_history_item)

        last_modified_timestamp = d.pop("last_modified_timestamp", UNSET)

        resource = cls(
            field_id=field_id,
            candidate_name=candidate_name,
            candidate_location=candidate_location,
            ip=ip,
            port=port,
            architecture=architecture,
            active=active,
            active_nodes=active_nodes,
            memory=memory,
            vcpus=vcpus,
            vgpus=vgpus,
            cpu_percent=cpu_percent,
            memory_percent=memory_percent,
            gpu_percent=gpu_percent,
            aggregation_per_architecture=aggregation_per_architecture,
            virtualization=virtualization,
            supported_addons=supported_addons,
            csi_drivers=csi_drivers,
            cpu_history=cpu_history,
            memory_history=memory_history,
            last_modified_timestamp=last_modified_timestamp,
        )

        resource.additional_properties = d
        return resource

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

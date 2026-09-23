from __future__ import annotations

from collections.abc import Mapping
from typing import TYPE_CHECKING, Any, TypeVar

from attrs import define as _attrs_define
from attrs import field as _attrs_field

from ..types import UNSET, Unset

if TYPE_CHECKING:
    from ..models.job_instance import JobInstance


T = TypeVar("T", bound="JobInstanceAppend")


@_attrs_define
class JobInstanceAppend:
    """Body of `PUT /api/v1/jobs/{job_id}/{instance_id}`. Only the last
    element of `instance_list` is appended; earlier elements are ignored.

        Attributes:
            instance_list (list[JobInstance] | Unset):
    """

    instance_list: list[JobInstance] | Unset = UNSET
    additional_properties: dict[str, Any] = _attrs_field(init=False, factory=dict)

    def to_dict(self) -> dict[str, Any]:
        instance_list: list[dict[str, Any]] | Unset = UNSET
        if not isinstance(self.instance_list, Unset):
            instance_list = []
            for instance_list_item_data in self.instance_list:
                instance_list_item = instance_list_item_data.to_dict()
                instance_list.append(instance_list_item)

        field_dict: dict[str, Any] = {}
        field_dict.update(self.additional_properties)
        field_dict.update({})
        if instance_list is not UNSET:
            field_dict["instance_list"] = instance_list

        return field_dict

    @classmethod
    def from_dict(cls: type[T], src_dict: Mapping[str, Any]) -> T:
        from ..models.job_instance import JobInstance

        d = dict(src_dict)
        _instance_list = d.pop("instance_list", UNSET)
        instance_list: list[JobInstance] | Unset = UNSET
        if _instance_list is not UNSET:
            instance_list = []
            for instance_list_item_data in _instance_list:
                instance_list_item = JobInstance.from_dict(instance_list_item_data)

                instance_list.append(instance_list_item)

        job_instance_append = cls(
            instance_list=instance_list,
        )

        job_instance_append.additional_properties = d
        return job_instance_append

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

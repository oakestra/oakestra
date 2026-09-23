from __future__ import annotations

from collections.abc import Mapping
from typing import TYPE_CHECKING, Any, TypeVar

from attrs import define as _attrs_define
from attrs import field as _attrs_field

from ..types import UNSET, Unset

if TYPE_CHECKING:
    from ..models.job_instance import JobInstance


T = TypeVar("T", bound="Job")


@_attrs_define
class Job:
    """A deployed microservice and its per-worker instances. Jobs carry the
    whole SLA the user submitted, so unknown fields are the norm here -
    they are stored and returned verbatim.

        Attributes:
            field_id (str | Unset):  Example: 65f1c0d2a1b2c3d4e5f60718.
            job_name (str | Unset): Unique per deployment; the key `PUT /api/v1/jobs` upserts on.
            application_id (str | Unset):
            candidate (str | Unset): Id of the candidate resource this job is placed on.
            instance_list (list[JobInstance] | Unset):
    """

    field_id: str | Unset = UNSET
    job_name: str | Unset = UNSET
    application_id: str | Unset = UNSET
    candidate: str | Unset = UNSET
    instance_list: list[JobInstance] | Unset = UNSET
    additional_properties: dict[str, Any] = _attrs_field(init=False, factory=dict)

    def to_dict(self) -> dict[str, Any]:
        field_id = self.field_id

        job_name = self.job_name

        application_id = self.application_id

        candidate = self.candidate

        instance_list: list[dict[str, Any]] | Unset = UNSET
        if not isinstance(self.instance_list, Unset):
            instance_list = []
            for instance_list_item_data in self.instance_list:
                instance_list_item = instance_list_item_data.to_dict()
                instance_list.append(instance_list_item)

        field_dict: dict[str, Any] = {}
        field_dict.update(self.additional_properties)
        field_dict.update({})
        if field_id is not UNSET:
            field_dict["_id"] = field_id
        if job_name is not UNSET:
            field_dict["job_name"] = job_name
        if application_id is not UNSET:
            field_dict["applicationID"] = application_id
        if candidate is not UNSET:
            field_dict["candidate"] = candidate
        if instance_list is not UNSET:
            field_dict["instance_list"] = instance_list

        return field_dict

    @classmethod
    def from_dict(cls: type[T], src_dict: Mapping[str, Any]) -> T:
        from ..models.job_instance import JobInstance

        d = dict(src_dict)
        field_id = d.pop("_id", UNSET)

        job_name = d.pop("job_name", UNSET)

        application_id = d.pop("applicationID", UNSET)

        candidate = d.pop("candidate", UNSET)

        _instance_list = d.pop("instance_list", UNSET)
        instance_list: list[JobInstance] | Unset = UNSET
        if _instance_list is not UNSET:
            instance_list = []
            for instance_list_item_data in _instance_list:
                instance_list_item = JobInstance.from_dict(instance_list_item_data)

                instance_list.append(instance_list_item)

        job = cls(
            field_id=field_id,
            job_name=job_name,
            application_id=application_id,
            candidate=candidate,
            instance_list=instance_list,
        )

        job.additional_properties = d
        return job

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

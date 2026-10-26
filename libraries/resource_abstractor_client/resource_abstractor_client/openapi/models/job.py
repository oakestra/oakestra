from __future__ import annotations

from collections.abc import Mapping
from typing import TYPE_CHECKING, Any, TypeVar, cast

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
            job_name (None | str | Unset): Unique per deployment; the key `PUT /api/v1/jobs` upserts on.
                Nullable because a `PATCH` sending an explicit `null` here $sets
                the stored value to null rather than leaving it untouched (only
                an absent key does that) - see `candidate`.
            application_id (None | str | Unset):
            candidate (None | str | Unset): Id of the candidate resource this job is placed on. Nullable: a
                `PATCH` body's `null` is written and read back as `null`,
                distinct from omitting the field entirely, which leaves the
                stored value alone.
            instance_list (list[JobInstance] | None | Unset):
    """

    field_id: str | Unset = UNSET
    job_name: None | str | Unset = UNSET
    application_id: None | str | Unset = UNSET
    candidate: None | str | Unset = UNSET
    instance_list: list[JobInstance] | None | Unset = UNSET
    additional_properties: dict[str, Any] = _attrs_field(init=False, factory=dict)

    def to_dict(self) -> dict[str, Any]:
        field_id = self.field_id

        job_name: None | str | Unset
        if isinstance(self.job_name, Unset):
            job_name = UNSET
        else:
            job_name = self.job_name

        application_id: None | str | Unset
        if isinstance(self.application_id, Unset):
            application_id = UNSET
        else:
            application_id = self.application_id

        candidate: None | str | Unset
        if isinstance(self.candidate, Unset):
            candidate = UNSET
        else:
            candidate = self.candidate

        instance_list: list[dict[str, Any]] | None | Unset
        if isinstance(self.instance_list, Unset):
            instance_list = UNSET
        elif isinstance(self.instance_list, list):
            instance_list = []
            for instance_list_type_0_item_data in self.instance_list:
                instance_list_type_0_item = instance_list_type_0_item_data.to_dict()
                instance_list.append(instance_list_type_0_item)

        else:
            instance_list = self.instance_list

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

        def _parse_job_name(data: object) -> None | str | Unset:
            if data is None:
                return data
            if isinstance(data, Unset):
                return data
            return cast(None | str | Unset, data)

        job_name = _parse_job_name(d.pop("job_name", UNSET))

        def _parse_application_id(data: object) -> None | str | Unset:
            if data is None:
                return data
            if isinstance(data, Unset):
                return data
            return cast(None | str | Unset, data)

        application_id = _parse_application_id(d.pop("applicationID", UNSET))

        def _parse_candidate(data: object) -> None | str | Unset:
            if data is None:
                return data
            if isinstance(data, Unset):
                return data
            return cast(None | str | Unset, data)

        candidate = _parse_candidate(d.pop("candidate", UNSET))

        def _parse_instance_list(data: object) -> list[JobInstance] | None | Unset:
            if data is None:
                return data
            if isinstance(data, Unset):
                return data
            try:
                if not isinstance(data, list):
                    raise TypeError()
                instance_list_type_0 = []
                _instance_list_type_0 = data
                for instance_list_type_0_item_data in _instance_list_type_0:
                    instance_list_type_0_item = JobInstance.from_dict(
                        instance_list_type_0_item_data
                    )

                    instance_list_type_0.append(instance_list_type_0_item)

                return instance_list_type_0
            except (TypeError, ValueError, AttributeError, KeyError):
                pass
            return cast(list[JobInstance] | None | Unset, data)

        instance_list = _parse_instance_list(d.pop("instance_list", UNSET))

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

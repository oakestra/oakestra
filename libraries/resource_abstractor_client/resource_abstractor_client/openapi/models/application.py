from __future__ import annotations

from collections.abc import Mapping
from typing import Any, TypeVar, cast

from attrs import define as _attrs_define
from attrs import field as _attrs_field

from ..types import UNSET, Unset

T = TypeVar("T", bound="Application")


@_attrs_define
class Application:
    """Application metadata. Unknown fields are stored and returned verbatim.

    Attributes:
        field_id (str | Unset):  Example: 65f1c0d2a1b2c3d4e5f60718.
        application_id (str | Unset): Set server-side on creation to the document's own `_id`.
        application_name (str | Unset):
        application_namespace (str | Unset):
        application_desc (str | Unset):
        user_id (str | Unset):
        microservices (list[str] | Unset):
    """

    field_id: str | Unset = UNSET
    application_id: str | Unset = UNSET
    application_name: str | Unset = UNSET
    application_namespace: str | Unset = UNSET
    application_desc: str | Unset = UNSET
    user_id: str | Unset = UNSET
    microservices: list[str] | Unset = UNSET
    additional_properties: dict[str, Any] = _attrs_field(init=False, factory=dict)

    def to_dict(self) -> dict[str, Any]:
        field_id = self.field_id

        application_id = self.application_id

        application_name = self.application_name

        application_namespace = self.application_namespace

        application_desc = self.application_desc

        user_id = self.user_id

        microservices: list[str] | Unset = UNSET
        if not isinstance(self.microservices, Unset):
            microservices = self.microservices

        field_dict: dict[str, Any] = {}
        field_dict.update(self.additional_properties)
        field_dict.update({})
        if field_id is not UNSET:
            field_dict["_id"] = field_id
        if application_id is not UNSET:
            field_dict["applicationID"] = application_id
        if application_name is not UNSET:
            field_dict["application_name"] = application_name
        if application_namespace is not UNSET:
            field_dict["application_namespace"] = application_namespace
        if application_desc is not UNSET:
            field_dict["application_desc"] = application_desc
        if user_id is not UNSET:
            field_dict["userId"] = user_id
        if microservices is not UNSET:
            field_dict["microservices"] = microservices

        return field_dict

    @classmethod
    def from_dict(cls: type[T], src_dict: Mapping[str, Any]) -> T:
        d = dict(src_dict)
        field_id = d.pop("_id", UNSET)

        application_id = d.pop("applicationID", UNSET)

        application_name = d.pop("application_name", UNSET)

        application_namespace = d.pop("application_namespace", UNSET)

        application_desc = d.pop("application_desc", UNSET)

        user_id = d.pop("userId", UNSET)

        microservices = cast(list[str], d.pop("microservices", UNSET))

        application = cls(
            field_id=field_id,
            application_id=application_id,
            application_name=application_name,
            application_namespace=application_namespace,
            application_desc=application_desc,
            user_id=user_id,
            microservices=microservices,
        )

        application.additional_properties = d
        return application

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

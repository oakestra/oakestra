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
        application_name (None | str | Unset): Nullable because a `PATCH` sending an explicit `null` here $sets
            the stored value to null rather than leaving it untouched (only
            an absent key does that).
        application_namespace (None | str | Unset):
        application_desc (None | str | Unset):
        user_id (None | str | Unset):
        microservices (list[str] | None | Unset):
    """

    field_id: str | Unset = UNSET
    application_id: str | Unset = UNSET
    application_name: None | str | Unset = UNSET
    application_namespace: None | str | Unset = UNSET
    application_desc: None | str | Unset = UNSET
    user_id: None | str | Unset = UNSET
    microservices: list[str] | None | Unset = UNSET
    additional_properties: dict[str, Any] = _attrs_field(init=False, factory=dict)

    def to_dict(self) -> dict[str, Any]:
        field_id = self.field_id

        application_id = self.application_id

        application_name: None | str | Unset
        if isinstance(self.application_name, Unset):
            application_name = UNSET
        else:
            application_name = self.application_name

        application_namespace: None | str | Unset
        if isinstance(self.application_namespace, Unset):
            application_namespace = UNSET
        else:
            application_namespace = self.application_namespace

        application_desc: None | str | Unset
        if isinstance(self.application_desc, Unset):
            application_desc = UNSET
        else:
            application_desc = self.application_desc

        user_id: None | str | Unset
        if isinstance(self.user_id, Unset):
            user_id = UNSET
        else:
            user_id = self.user_id

        microservices: list[str] | None | Unset
        if isinstance(self.microservices, Unset):
            microservices = UNSET
        elif isinstance(self.microservices, list):
            microservices = self.microservices

        else:
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

        def _parse_application_name(data: object) -> None | str | Unset:
            if data is None:
                return data
            if isinstance(data, Unset):
                return data
            return cast(None | str | Unset, data)

        application_name = _parse_application_name(d.pop("application_name", UNSET))

        def _parse_application_namespace(data: object) -> None | str | Unset:
            if data is None:
                return data
            if isinstance(data, Unset):
                return data
            return cast(None | str | Unset, data)

        application_namespace = _parse_application_namespace(d.pop("application_namespace", UNSET))

        def _parse_application_desc(data: object) -> None | str | Unset:
            if data is None:
                return data
            if isinstance(data, Unset):
                return data
            return cast(None | str | Unset, data)

        application_desc = _parse_application_desc(d.pop("application_desc", UNSET))

        def _parse_user_id(data: object) -> None | str | Unset:
            if data is None:
                return data
            if isinstance(data, Unset):
                return data
            return cast(None | str | Unset, data)

        user_id = _parse_user_id(d.pop("userId", UNSET))

        def _parse_microservices(data: object) -> list[str] | None | Unset:
            if data is None:
                return data
            if isinstance(data, Unset):
                return data
            try:
                if not isinstance(data, list):
                    raise TypeError()
                microservices_type_0 = cast(list[str], data)

                return microservices_type_0
            except (TypeError, ValueError, AttributeError, KeyError):
                pass
            return cast(list[str] | None | Unset, data)

        microservices = _parse_microservices(d.pop("microservices", UNSET))

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

from __future__ import annotations

from collections.abc import Mapping
from typing import TYPE_CHECKING, Any, TypeVar

from attrs import define as _attrs_define
from attrs import field as _attrs_field

from ..types import UNSET, Unset

if TYPE_CHECKING:
    from ..models.custom_resource_definition_schema import CustomResourceDefinitionSchema


T = TypeVar("T", bound="CustomResourceDefinition")


@_attrs_define
class CustomResourceDefinition:
    """A dynamically registered resource type.

    Attributes:
        resource_type (str): Names the type and the path segment its instances live under
            (`/api/v1/custom-resources/{resource_type}`).
        field_id (str | Unset):  Example: 65f1c0d2a1b2c3d4e5f60718.
        schema (CustomResourceDefinitionSchema | Unset): JSON Schema every instance of this type is validated against.
            An
            absent or empty schema imposes no constraint.
    """

    resource_type: str
    field_id: str | Unset = UNSET
    schema: CustomResourceDefinitionSchema | Unset = UNSET
    additional_properties: dict[str, Any] = _attrs_field(init=False, factory=dict)

    def to_dict(self) -> dict[str, Any]:
        resource_type = self.resource_type

        field_id = self.field_id

        schema: dict[str, Any] | Unset = UNSET
        if not isinstance(self.schema, Unset):
            schema = self.schema.to_dict()

        field_dict: dict[str, Any] = {}
        field_dict.update(self.additional_properties)
        field_dict.update(
            {
                "resource_type": resource_type,
            }
        )
        if field_id is not UNSET:
            field_dict["_id"] = field_id
        if schema is not UNSET:
            field_dict["schema"] = schema

        return field_dict

    @classmethod
    def from_dict(cls: type[T], src_dict: Mapping[str, Any]) -> T:
        from ..models.custom_resource_definition_schema import CustomResourceDefinitionSchema

        d = dict(src_dict)
        resource_type = d.pop("resource_type")

        field_id = d.pop("_id", UNSET)

        _schema = d.pop("schema", UNSET)
        schema: CustomResourceDefinitionSchema | Unset
        if isinstance(_schema, Unset):
            schema = UNSET
        else:
            schema = CustomResourceDefinitionSchema.from_dict(_schema)

        custom_resource_definition = cls(
            resource_type=resource_type,
            field_id=field_id,
            schema=schema,
        )

        custom_resource_definition.additional_properties = d
        return custom_resource_definition

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

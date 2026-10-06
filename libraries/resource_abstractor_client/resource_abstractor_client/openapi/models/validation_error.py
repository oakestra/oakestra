from __future__ import annotations

from collections.abc import Mapping
from typing import TYPE_CHECKING, Any, TypeVar

from attrs import define as _attrs_define
from attrs import field as _attrs_field

if TYPE_CHECKING:
    from ..models.validation_error_details import ValidationErrorDetails


T = TypeVar("T", bound="ValidationError")


@_attrs_define
class ValidationError:
    """A validation failure, carrying per-field messages under `details`. The
    shape of `details` follows the failing location: `{"body": {...}}` for
    request bodies and `{"query": {...}}` for query parameters.

        Example:
            {'message': 'Invalid input', 'details': {'query': {'active': ['active must be a valid boolean']}}}

        Attributes:
            message (str):  Example: Invalid input.
            details (ValidationErrorDetails):
    """

    message: str
    details: ValidationErrorDetails
    additional_properties: dict[str, Any] = _attrs_field(init=False, factory=dict)

    def to_dict(self) -> dict[str, Any]:
        message = self.message

        details = self.details.to_dict()

        field_dict: dict[str, Any] = {}
        field_dict.update(self.additional_properties)
        field_dict.update(
            {
                "message": message,
                "details": details,
            }
        )

        return field_dict

    @classmethod
    def from_dict(cls: type[T], src_dict: Mapping[str, Any]) -> T:
        from ..models.validation_error_details import ValidationErrorDetails

        d = dict(src_dict)
        message = d.pop("message")

        details = ValidationErrorDetails.from_dict(d.pop("details"))

        validation_error = cls(
            message=message,
            details=details,
        )

        validation_error.additional_properties = d
        return validation_error

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

from __future__ import annotations

from collections.abc import Mapping
from typing import Any, TypeVar

from attrs import define as _attrs_define
from attrs import field as _attrs_field

from ..models.hook_event import HookEvent
from ..types import UNSET, Unset

T = TypeVar("T", bound="Hook")


@_attrs_define
class Hook:
    """A webhook registration.

    Attributes:
        field_id (str | Unset):  Example: 65f1c0d2a1b2c3d4e5f60718.
        hook_name (str | Unset):
        webhook_url (str | Unset):
        entity (str | Unset): The entity to fire for: `resources`, `applications`, `jobs`, or a
            registered custom `resource_type`.
        events (list[HookEvent] | Unset):
    """

    field_id: str | Unset = UNSET
    hook_name: str | Unset = UNSET
    webhook_url: str | Unset = UNSET
    entity: str | Unset = UNSET
    events: list[HookEvent] | Unset = UNSET
    additional_properties: dict[str, Any] = _attrs_field(init=False, factory=dict)

    def to_dict(self) -> dict[str, Any]:
        field_id = self.field_id

        hook_name = self.hook_name

        webhook_url = self.webhook_url

        entity = self.entity

        events: list[str] | Unset = UNSET
        if not isinstance(self.events, Unset):
            events = []
            for events_item_data in self.events:
                events_item = events_item_data.value
                events.append(events_item)

        field_dict: dict[str, Any] = {}
        field_dict.update(self.additional_properties)
        field_dict.update({})
        if field_id is not UNSET:
            field_dict["_id"] = field_id
        if hook_name is not UNSET:
            field_dict["hook_name"] = hook_name
        if webhook_url is not UNSET:
            field_dict["webhook_url"] = webhook_url
        if entity is not UNSET:
            field_dict["entity"] = entity
        if events is not UNSET:
            field_dict["events"] = events

        return field_dict

    @classmethod
    def from_dict(cls: type[T], src_dict: Mapping[str, Any]) -> T:
        d = dict(src_dict)
        field_id = d.pop("_id", UNSET)

        hook_name = d.pop("hook_name", UNSET)

        webhook_url = d.pop("webhook_url", UNSET)

        entity = d.pop("entity", UNSET)

        _events = d.pop("events", UNSET)
        events: list[HookEvent] | Unset = UNSET
        if _events is not UNSET:
            events = []
            for events_item_data in _events:
                events_item = HookEvent(events_item_data)

                events.append(events_item)

        hook = cls(
            field_id=field_id,
            hook_name=hook_name,
            webhook_url=webhook_url,
            entity=entity,
            events=events,
        )

        hook.additional_properties = d
        return hook

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

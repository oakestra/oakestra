from google.protobuf.internal import containers as _containers
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from typing import ClassVar as _ClassVar, Iterable as _Iterable, Mapping as _Mapping, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class CS1Message(_message.Message):
    __slots__ = ("hello_service_manager",)
    HELLO_SERVICE_MANAGER_FIELD_NUMBER: _ClassVar[int]
    hello_service_manager: str
    def __init__(self, hello_service_manager: _Optional[str] = ...) -> None: ...

class SC1Message(_message.Message):
    __slots__ = ("hello_cluster_manager",)
    HELLO_CLUSTER_MANAGER_FIELD_NUMBER: _ClassVar[int]
    hello_cluster_manager: str
    def __init__(self, hello_cluster_manager: _Optional[str] = ...) -> None: ...

class CS2Message(_message.Message):
    __slots__ = ("manager_port", "network_component_port", "cluster_name", "cluster_info", "cluster_location", "cluster_ip", "token")
    MANAGER_PORT_FIELD_NUMBER: _ClassVar[int]
    NETWORK_COMPONENT_PORT_FIELD_NUMBER: _ClassVar[int]
    CLUSTER_NAME_FIELD_NUMBER: _ClassVar[int]
    CLUSTER_INFO_FIELD_NUMBER: _ClassVar[int]
    CLUSTER_LOCATION_FIELD_NUMBER: _ClassVar[int]
    CLUSTER_IP_FIELD_NUMBER: _ClassVar[int]
    TOKEN_FIELD_NUMBER: _ClassVar[int]
    manager_port: int
    network_component_port: int
    cluster_name: str
    cluster_info: _containers.RepeatedCompositeFieldContainer[KeyValue]
    cluster_location: str
    cluster_ip: str
    token: str
    def __init__(self, manager_port: _Optional[int] = ..., network_component_port: _Optional[int] = ..., cluster_name: _Optional[str] = ..., cluster_info: _Optional[_Iterable[_Union[KeyValue, _Mapping]]] = ..., cluster_location: _Optional[str] = ..., cluster_ip: _Optional[str] = ..., token: _Optional[str] = ...) -> None: ...

class KeyValue(_message.Message):
    __slots__ = ("key", "value")
    KEY_FIELD_NUMBER: _ClassVar[int]
    VALUE_FIELD_NUMBER: _ClassVar[int]
    key: str
    value: str
    def __init__(self, key: _Optional[str] = ..., value: _Optional[str] = ...) -> None: ...

class SC2Message(_message.Message):
    __slots__ = ("id",)
    ID_FIELD_NUMBER: _ClassVar[int]
    id: str
    def __init__(self, id: _Optional[str] = ...) -> None: ...

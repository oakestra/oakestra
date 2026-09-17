"""Contains all the data models used in inputs/outputs"""

from .application import Application
from .custom_resource_definition import CustomResourceDefinition
from .custom_resource_definition_schema import CustomResourceDefinitionSchema
from .custom_resource_instance import CustomResourceInstance
from .deleted_id import DeletedID
from .get_spec_json_response_200 import GetSpecJSONResponse200
from .history_sample import HistorySample
from .hook import Hook
from .hook_event import HookEvent
from .job import Job
from .job_history_sample import JobHistorySample
from .job_instance import JobInstance
from .job_instance_append import JobInstanceAppend
from .message import Message
from .resource import Resource
from .resource_aggregation_per_architecture import ResourceAggregationPerArchitecture
from .validation_error import ValidationError
from .validation_error_details import ValidationErrorDetails

__all__ = (
    "Application",
    "CustomResourceDefinition",
    "CustomResourceDefinitionSchema",
    "CustomResourceInstance",
    "DeletedID",
    "GetSpecJSONResponse200",
    "HistorySample",
    "Hook",
    "HookEvent",
    "Job",
    "JobHistorySample",
    "JobInstance",
    "JobInstanceAppend",
    "Message",
    "Resource",
    "ResourceAggregationPerArchitecture",
    "ValidationError",
    "ValidationErrorDetails",
)

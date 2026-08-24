from http import HTTPStatus
from typing import Any

import httpx

from ... import errors
from ...client import AuthenticatedClient, Client
from ...models.message import Message
from ...models.resource import Resource
from ...models.validation_error import ValidationError
from ...types import UNSET, Response, Unset


def _get_kwargs(
    *,
    active: str | Unset = UNSET,
    job_id: str | Unset = UNSET,
    candidate_name: str | Unset = UNSET,
    ip: str | Unset = UNSET,
    resources: list[str] | Unset = UNSET,
) -> dict[str, Any]:

    params: dict[str, Any] = {}

    params["active"] = active

    params["job_id"] = job_id

    params["candidate_name"] = candidate_name

    params["ip"] = ip

    json_resources: list[str] | Unset = UNSET
    if not isinstance(resources, Unset):
        json_resources = resources

    params["resources"] = json_resources

    params = {k: v for k, v in params.items() if v is not UNSET and v is not None}

    _kwargs: dict[str, Any] = {
        "method": "get",
        "url": "/api/v1/resources",
        "params": params,
    }

    return _kwargs


def _parse_response(
    *, client: AuthenticatedClient | Client, response: httpx.Response
) -> Message | ValidationError | list[Resource] | None:
    if response.status_code == 200:
        response_200 = []
        _response_200 = response.json()
        for response_200_item_data in _response_200:
            response_200_item = Resource.from_dict(response_200_item_data)

            response_200.append(response_200_item)

        return response_200

    if response.status_code == 400:
        response_400 = Message.from_dict(response.json())

        return response_400

    if response.status_code == 404:
        response_404 = Message.from_dict(response.json())

        return response_404

    if response.status_code == 422:
        response_422 = ValidationError.from_dict(response.json())

        return response_422

    if client.raise_on_unexpected_status:
        raise errors.UnexpectedStatus(response.status_code, response.content)
    else:
        return None


def _build_response(
    *, client: AuthenticatedClient | Client, response: httpx.Response
) -> Response[Message | ValidationError | list[Resource]]:
    return Response(
        status_code=HTTPStatus(response.status_code),
        content=response.content,
        headers=response.headers,
        parsed=_parse_response(client=client, response=response),
    )


def sync_detailed(
    *,
    client: AuthenticatedClient | Client,
    active: str | Unset = UNSET,
    job_id: str | Unset = UNSET,
    candidate_name: str | Unset = UNSET,
    ip: str | Unset = UNSET,
    resources: list[str] | Unset = UNSET,
) -> Response[Message | ValidationError | list[Resource]]:
    """List candidate resources

     Returns candidates with a computed `active` flag (true when
    `last_modified_timestamp` is within the last 30 seconds) and a
    projection limited to the canonical resource fields, extendable via
    `resources`.

    Args:
        active (str | Unset):
        job_id (str | Unset):  Example: 65f1c0d2a1b2c3d4e5f60718.
        candidate_name (str | Unset):
        ip (str | Unset):
        resources (list[str] | Unset):

    Raises:
        errors.UnexpectedStatus: If the server returns an undocumented status code and Client.raise_on_unexpected_status is True.
        httpx.TimeoutException: If the request takes longer than Client.timeout.

    Returns:
        Response[Message | ValidationError | list[Resource]]
    """

    kwargs = _get_kwargs(
        active=active,
        job_id=job_id,
        candidate_name=candidate_name,
        ip=ip,
        resources=resources,
    )

    response = client.get_httpx_client().request(
        **kwargs,
    )

    return _build_response(client=client, response=response)


def sync(
    *,
    client: AuthenticatedClient | Client,
    active: str | Unset = UNSET,
    job_id: str | Unset = UNSET,
    candidate_name: str | Unset = UNSET,
    ip: str | Unset = UNSET,
    resources: list[str] | Unset = UNSET,
) -> Message | ValidationError | list[Resource] | None:
    """List candidate resources

     Returns candidates with a computed `active` flag (true when
    `last_modified_timestamp` is within the last 30 seconds) and a
    projection limited to the canonical resource fields, extendable via
    `resources`.

    Args:
        active (str | Unset):
        job_id (str | Unset):  Example: 65f1c0d2a1b2c3d4e5f60718.
        candidate_name (str | Unset):
        ip (str | Unset):
        resources (list[str] | Unset):

    Raises:
        errors.UnexpectedStatus: If the server returns an undocumented status code and Client.raise_on_unexpected_status is True.
        httpx.TimeoutException: If the request takes longer than Client.timeout.

    Returns:
        Message | ValidationError | list[Resource]
    """

    return sync_detailed(
        client=client,
        active=active,
        job_id=job_id,
        candidate_name=candidate_name,
        ip=ip,
        resources=resources,
    ).parsed


async def asyncio_detailed(
    *,
    client: AuthenticatedClient | Client,
    active: str | Unset = UNSET,
    job_id: str | Unset = UNSET,
    candidate_name: str | Unset = UNSET,
    ip: str | Unset = UNSET,
    resources: list[str] | Unset = UNSET,
) -> Response[Message | ValidationError | list[Resource]]:
    """List candidate resources

     Returns candidates with a computed `active` flag (true when
    `last_modified_timestamp` is within the last 30 seconds) and a
    projection limited to the canonical resource fields, extendable via
    `resources`.

    Args:
        active (str | Unset):
        job_id (str | Unset):  Example: 65f1c0d2a1b2c3d4e5f60718.
        candidate_name (str | Unset):
        ip (str | Unset):
        resources (list[str] | Unset):

    Raises:
        errors.UnexpectedStatus: If the server returns an undocumented status code and Client.raise_on_unexpected_status is True.
        httpx.TimeoutException: If the request takes longer than Client.timeout.

    Returns:
        Response[Message | ValidationError | list[Resource]]
    """

    kwargs = _get_kwargs(
        active=active,
        job_id=job_id,
        candidate_name=candidate_name,
        ip=ip,
        resources=resources,
    )

    response = await client.get_async_httpx_client().request(**kwargs)

    return _build_response(client=client, response=response)


async def asyncio(
    *,
    client: AuthenticatedClient | Client,
    active: str | Unset = UNSET,
    job_id: str | Unset = UNSET,
    candidate_name: str | Unset = UNSET,
    ip: str | Unset = UNSET,
    resources: list[str] | Unset = UNSET,
) -> Message | ValidationError | list[Resource] | None:
    """List candidate resources

     Returns candidates with a computed `active` flag (true when
    `last_modified_timestamp` is within the last 30 seconds) and a
    projection limited to the canonical resource fields, extendable via
    `resources`.

    Args:
        active (str | Unset):
        job_id (str | Unset):  Example: 65f1c0d2a1b2c3d4e5f60718.
        candidate_name (str | Unset):
        ip (str | Unset):
        resources (list[str] | Unset):

    Raises:
        errors.UnexpectedStatus: If the server returns an undocumented status code and Client.raise_on_unexpected_status is True.
        httpx.TimeoutException: If the request takes longer than Client.timeout.

    Returns:
        Message | ValidationError | list[Resource]
    """

    return (
        await asyncio_detailed(
            client=client,
            active=active,
            job_id=job_id,
            candidate_name=candidate_name,
            ip=ip,
            resources=resources,
        )
    ).parsed

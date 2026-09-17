from http import HTTPStatus
from typing import Any
from urllib.parse import quote

import httpx

from ... import errors
from ...client import AuthenticatedClient, Client
from ...models.message import Message
from ...models.resource import Resource
from ...models.validation_error import ValidationError
from ...types import UNSET, Response, Unset


def _get_kwargs(
    id: str,
    *,
    body: Resource | Unset = UNSET,
) -> dict[str, Any]:
    headers: dict[str, Any] = {}

    _kwargs: dict[str, Any] = {
        "method": "patch",
        "url": "/api/v1/resources/{id}".format(
            id=quote(str(id), safe=""),
        ),
    }

    if not isinstance(body, Unset):
        _kwargs["json"] = body.to_dict()

    headers["Content-Type"] = "application/json"

    _kwargs["headers"] = headers
    return _kwargs


def _parse_response(
    *, client: AuthenticatedClient | Client, response: httpx.Response
) -> Message | Resource | ValidationError | None:
    if response.status_code == 200:
        response_200 = Resource.from_dict(response.json())

        return response_200

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
) -> Response[Message | Resource | ValidationError]:
    return Response(
        status_code=HTTPStatus(response.status_code),
        content=response.content,
        headers=response.headers,
        parsed=_parse_response(client=client, response=response),
    )


def sync_detailed(
    id: str,
    *,
    client: AuthenticatedClient | Client,
    body: Resource | Unset = UNSET,
) -> Response[Message | Resource | ValidationError]:
    """Report resource usage for a candidate

     Persists an aggregated usage report: `last_modified` /
    `last_modified_timestamp` are refreshed server-side, the reported
    `cpu_percent` and `memory_percent` are appended to `cpu_history` and
    `memory_history` (each capped at the most recent 100 samples), and the
    rest of the body is stored as-is.

    Any `cpu_history` / `memory_history` sent by the client is ignored, so
    echoing a previously read document back does not duplicate history.

    Note that an `id` that is not a valid ObjectID is reported as 404 here,
    not 400 - matching the original Flask service.

    Args:
        id (str):  Example: 65f1c0d2a1b2c3d4e5f60718.
        body (Resource | Unset): A candidate resource: a worker node at cluster level, a whole
            cluster
            at root level. Unknown fields are stored and returned verbatim.

    Raises:
        errors.UnexpectedStatus: If the server returns an undocumented status code and Client.raise_on_unexpected_status is True.
        httpx.TimeoutException: If the request takes longer than Client.timeout.

    Returns:
        Response[Message | Resource | ValidationError]
    """

    kwargs = _get_kwargs(
        id=id,
        body=body,
    )

    response = client.get_httpx_client().request(
        **kwargs,
    )

    return _build_response(client=client, response=response)


def sync(
    id: str,
    *,
    client: AuthenticatedClient | Client,
    body: Resource | Unset = UNSET,
) -> Message | Resource | ValidationError | None:
    """Report resource usage for a candidate

     Persists an aggregated usage report: `last_modified` /
    `last_modified_timestamp` are refreshed server-side, the reported
    `cpu_percent` and `memory_percent` are appended to `cpu_history` and
    `memory_history` (each capped at the most recent 100 samples), and the
    rest of the body is stored as-is.

    Any `cpu_history` / `memory_history` sent by the client is ignored, so
    echoing a previously read document back does not duplicate history.

    Note that an `id` that is not a valid ObjectID is reported as 404 here,
    not 400 - matching the original Flask service.

    Args:
        id (str):  Example: 65f1c0d2a1b2c3d4e5f60718.
        body (Resource | Unset): A candidate resource: a worker node at cluster level, a whole
            cluster
            at root level. Unknown fields are stored and returned verbatim.

    Raises:
        errors.UnexpectedStatus: If the server returns an undocumented status code and Client.raise_on_unexpected_status is True.
        httpx.TimeoutException: If the request takes longer than Client.timeout.

    Returns:
        Message | Resource | ValidationError
    """

    return sync_detailed(
        id=id,
        client=client,
        body=body,
    ).parsed


async def asyncio_detailed(
    id: str,
    *,
    client: AuthenticatedClient | Client,
    body: Resource | Unset = UNSET,
) -> Response[Message | Resource | ValidationError]:
    """Report resource usage for a candidate

     Persists an aggregated usage report: `last_modified` /
    `last_modified_timestamp` are refreshed server-side, the reported
    `cpu_percent` and `memory_percent` are appended to `cpu_history` and
    `memory_history` (each capped at the most recent 100 samples), and the
    rest of the body is stored as-is.

    Any `cpu_history` / `memory_history` sent by the client is ignored, so
    echoing a previously read document back does not duplicate history.

    Note that an `id` that is not a valid ObjectID is reported as 404 here,
    not 400 - matching the original Flask service.

    Args:
        id (str):  Example: 65f1c0d2a1b2c3d4e5f60718.
        body (Resource | Unset): A candidate resource: a worker node at cluster level, a whole
            cluster
            at root level. Unknown fields are stored and returned verbatim.

    Raises:
        errors.UnexpectedStatus: If the server returns an undocumented status code and Client.raise_on_unexpected_status is True.
        httpx.TimeoutException: If the request takes longer than Client.timeout.

    Returns:
        Response[Message | Resource | ValidationError]
    """

    kwargs = _get_kwargs(
        id=id,
        body=body,
    )

    response = await client.get_async_httpx_client().request(**kwargs)

    return _build_response(client=client, response=response)


async def asyncio(
    id: str,
    *,
    client: AuthenticatedClient | Client,
    body: Resource | Unset = UNSET,
) -> Message | Resource | ValidationError | None:
    """Report resource usage for a candidate

     Persists an aggregated usage report: `last_modified` /
    `last_modified_timestamp` are refreshed server-side, the reported
    `cpu_percent` and `memory_percent` are appended to `cpu_history` and
    `memory_history` (each capped at the most recent 100 samples), and the
    rest of the body is stored as-is.

    Any `cpu_history` / `memory_history` sent by the client is ignored, so
    echoing a previously read document back does not duplicate history.

    Note that an `id` that is not a valid ObjectID is reported as 404 here,
    not 400 - matching the original Flask service.

    Args:
        id (str):  Example: 65f1c0d2a1b2c3d4e5f60718.
        body (Resource | Unset): A candidate resource: a worker node at cluster level, a whole
            cluster
            at root level. Unknown fields are stored and returned verbatim.

    Raises:
        errors.UnexpectedStatus: If the server returns an undocumented status code and Client.raise_on_unexpected_status is True.
        httpx.TimeoutException: If the request takes longer than Client.timeout.

    Returns:
        Message | Resource | ValidationError
    """

    return (
        await asyncio_detailed(
            id=id,
            client=client,
            body=body,
        )
    ).parsed

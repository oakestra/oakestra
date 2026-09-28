from http import HTTPStatus
from typing import Any
from urllib.parse import quote

import httpx

from ... import errors
from ...client import AuthenticatedClient, Client
from ...models.hook import Hook
from ...models.message import Message
from ...types import UNSET, Response, Unset


def _get_kwargs(
    id: str,
    *,
    body: Hook | Unset = UNSET,
) -> dict[str, Any]:
    headers: dict[str, Any] = {}

    _kwargs: dict[str, Any] = {
        "method": "patch",
        "url": "/api/v1/hooks/{id}".format(
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
) -> Hook | Message | None:
    if response.status_code == 200:
        response_200 = Hook.from_dict(response.json())

        return response_200

    if response.status_code == 400:
        response_400 = Message.from_dict(response.json())

        return response_400

    if response.status_code == 404:
        response_404 = Message.from_dict(response.json())

        return response_404

    if client.raise_on_unexpected_status:
        raise errors.UnexpectedStatus(response.status_code, response.content)
    else:
        return None


def _build_response(
    *, client: AuthenticatedClient | Client, response: httpx.Response
) -> Response[Hook | Message]:
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
    body: Hook | Unset = UNSET,
) -> Response[Hook | Message]:
    """Update a webhook registration

    Args:
        id (str):  Example: 65f1c0d2a1b2c3d4e5f60718.
        body (Hook | Unset): A webhook registration.

    Raises:
        errors.UnexpectedStatus: If the server returns an undocumented status code and Client.raise_on_unexpected_status is True.
        httpx.TimeoutException: If the request takes longer than Client.timeout.

    Returns:
        Response[Hook | Message]
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
    body: Hook | Unset = UNSET,
) -> Hook | Message | None:
    """Update a webhook registration

    Args:
        id (str):  Example: 65f1c0d2a1b2c3d4e5f60718.
        body (Hook | Unset): A webhook registration.

    Raises:
        errors.UnexpectedStatus: If the server returns an undocumented status code and Client.raise_on_unexpected_status is True.
        httpx.TimeoutException: If the request takes longer than Client.timeout.

    Returns:
        Hook | Message
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
    body: Hook | Unset = UNSET,
) -> Response[Hook | Message]:
    """Update a webhook registration

    Args:
        id (str):  Example: 65f1c0d2a1b2c3d4e5f60718.
        body (Hook | Unset): A webhook registration.

    Raises:
        errors.UnexpectedStatus: If the server returns an undocumented status code and Client.raise_on_unexpected_status is True.
        httpx.TimeoutException: If the request takes longer than Client.timeout.

    Returns:
        Response[Hook | Message]
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
    body: Hook | Unset = UNSET,
) -> Hook | Message | None:
    """Update a webhook registration

    Args:
        id (str):  Example: 65f1c0d2a1b2c3d4e5f60718.
        body (Hook | Unset): A webhook registration.

    Raises:
        errors.UnexpectedStatus: If the server returns an undocumented status code and Client.raise_on_unexpected_status is True.
        httpx.TimeoutException: If the request takes longer than Client.timeout.

    Returns:
        Hook | Message
    """

    return (
        await asyncio_detailed(
            id=id,
            client=client,
            body=body,
        )
    ).parsed

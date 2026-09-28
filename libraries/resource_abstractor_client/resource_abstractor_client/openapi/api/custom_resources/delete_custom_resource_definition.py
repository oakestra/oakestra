from http import HTTPStatus
from typing import Any
from urllib.parse import quote

import httpx

from ... import errors
from ...client import AuthenticatedClient, Client
from ...models.message import Message
from ...types import Response


def _get_kwargs(
    resource: str,
) -> dict[str, Any]:

    _kwargs: dict[str, Any] = {
        "method": "delete",
        "url": "/api/v1/custom-resources/{resource}".format(
            resource=quote(str(resource), safe=""),
        ),
    }

    return _kwargs


def _parse_response(
    *, client: AuthenticatedClient | Client, response: httpx.Response
) -> Message | None:
    if response.status_code == 200:
        response_200 = Message.from_dict(response.json())

        return response_200

    if response.status_code == 404:
        response_404 = Message.from_dict(response.json())

        return response_404

    if client.raise_on_unexpected_status:
        raise errors.UnexpectedStatus(response.status_code, response.content)
    else:
        return None


def _build_response(
    *, client: AuthenticatedClient | Client, response: httpx.Response
) -> Response[Message]:
    return Response(
        status_code=HTTPStatus(response.status_code),
        content=response.content,
        headers=response.headers,
        parsed=_parse_response(client=client, response=response),
    )


def sync_detailed(
    resource: str,
    *,
    client: AuthenticatedClient | Client,
) -> Response[Message]:
    """Delete a custom resource type and every instance of it

     A cascading delete: all instances first, then the definition.

    Args:
        resource (str):

    Raises:
        errors.UnexpectedStatus: If the server returns an undocumented status code and Client.raise_on_unexpected_status is True.
        httpx.TimeoutException: If the request takes longer than Client.timeout.

    Returns:
        Response[Message]
    """

    kwargs = _get_kwargs(
        resource=resource,
    )

    response = client.get_httpx_client().request(
        **kwargs,
    )

    return _build_response(client=client, response=response)


def sync(
    resource: str,
    *,
    client: AuthenticatedClient | Client,
) -> Message | None:
    """Delete a custom resource type and every instance of it

     A cascading delete: all instances first, then the definition.

    Args:
        resource (str):

    Raises:
        errors.UnexpectedStatus: If the server returns an undocumented status code and Client.raise_on_unexpected_status is True.
        httpx.TimeoutException: If the request takes longer than Client.timeout.

    Returns:
        Message
    """

    return sync_detailed(
        resource=resource,
        client=client,
    ).parsed


async def asyncio_detailed(
    resource: str,
    *,
    client: AuthenticatedClient | Client,
) -> Response[Message]:
    """Delete a custom resource type and every instance of it

     A cascading delete: all instances first, then the definition.

    Args:
        resource (str):

    Raises:
        errors.UnexpectedStatus: If the server returns an undocumented status code and Client.raise_on_unexpected_status is True.
        httpx.TimeoutException: If the request takes longer than Client.timeout.

    Returns:
        Response[Message]
    """

    kwargs = _get_kwargs(
        resource=resource,
    )

    response = await client.get_async_httpx_client().request(**kwargs)

    return _build_response(client=client, response=response)


async def asyncio(
    resource: str,
    *,
    client: AuthenticatedClient | Client,
) -> Message | None:
    """Delete a custom resource type and every instance of it

     A cascading delete: all instances first, then the definition.

    Args:
        resource (str):

    Raises:
        errors.UnexpectedStatus: If the server returns an undocumented status code and Client.raise_on_unexpected_status is True.
        httpx.TimeoutException: If the request takes longer than Client.timeout.

    Returns:
        Message
    """

    return (
        await asyncio_detailed(
            resource=resource,
            client=client,
        )
    ).parsed

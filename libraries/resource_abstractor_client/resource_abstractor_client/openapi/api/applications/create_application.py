from http import HTTPStatus
from typing import Any

import httpx

from ... import errors
from ...client import AuthenticatedClient, Client
from ...models.application import Application
from ...models.message import Message
from ...types import Response


def _get_kwargs(
    *,
    body: Application,
) -> dict[str, Any]:
    headers: dict[str, Any] = {}

    _kwargs: dict[str, Any] = {
        "method": "post",
        "url": "/api/v1/applications",
    }

    _kwargs["json"] = body.to_dict()

    headers["Content-Type"] = "application/json"

    _kwargs["headers"] = headers
    return _kwargs


def _parse_response(
    *, client: AuthenticatedClient | Client, response: httpx.Response
) -> Application | Message | None:
    if response.status_code == 200:
        response_200 = Application.from_dict(response.json())

        return response_200

    if response.status_code == 400:
        response_400 = Message.from_dict(response.json())

        return response_400

    if client.raise_on_unexpected_status:
        raise errors.UnexpectedStatus(response.status_code, response.content)
    else:
        return None


def _build_response(
    *, client: AuthenticatedClient | Client, response: httpx.Response
) -> Response[Application | Message]:
    return Response(
        status_code=HTTPStatus(response.status_code),
        content=response.content,
        headers=response.headers,
        parsed=_parse_response(client=client, response=response),
    )


def sync_detailed(
    *,
    client: AuthenticatedClient | Client,
    body: Application,
) -> Response[Application | Message]:
    """Create an application

     The stored document's `applicationID` is populated server-side with the
    new document's own stringified `_id`.

    Args:
        body (Application): Application metadata. Unknown fields are stored and returned verbatim.

    Raises:
        errors.UnexpectedStatus: If the server returns an undocumented status code and Client.raise_on_unexpected_status is True.
        httpx.TimeoutException: If the request takes longer than Client.timeout.

    Returns:
        Response[Application | Message]
    """

    kwargs = _get_kwargs(
        body=body,
    )

    response = client.get_httpx_client().request(
        **kwargs,
    )

    return _build_response(client=client, response=response)


def sync(
    *,
    client: AuthenticatedClient | Client,
    body: Application,
) -> Application | Message | None:
    """Create an application

     The stored document's `applicationID` is populated server-side with the
    new document's own stringified `_id`.

    Args:
        body (Application): Application metadata. Unknown fields are stored and returned verbatim.

    Raises:
        errors.UnexpectedStatus: If the server returns an undocumented status code and Client.raise_on_unexpected_status is True.
        httpx.TimeoutException: If the request takes longer than Client.timeout.

    Returns:
        Application | Message
    """

    return sync_detailed(
        client=client,
        body=body,
    ).parsed


async def asyncio_detailed(
    *,
    client: AuthenticatedClient | Client,
    body: Application,
) -> Response[Application | Message]:
    """Create an application

     The stored document's `applicationID` is populated server-side with the
    new document's own stringified `_id`.

    Args:
        body (Application): Application metadata. Unknown fields are stored and returned verbatim.

    Raises:
        errors.UnexpectedStatus: If the server returns an undocumented status code and Client.raise_on_unexpected_status is True.
        httpx.TimeoutException: If the request takes longer than Client.timeout.

    Returns:
        Response[Application | Message]
    """

    kwargs = _get_kwargs(
        body=body,
    )

    response = await client.get_async_httpx_client().request(**kwargs)

    return _build_response(client=client, response=response)


async def asyncio(
    *,
    client: AuthenticatedClient | Client,
    body: Application,
) -> Application | Message | None:
    """Create an application

     The stored document's `applicationID` is populated server-side with the
    new document's own stringified `_id`.

    Args:
        body (Application): Application metadata. Unknown fields are stored and returned verbatim.

    Raises:
        errors.UnexpectedStatus: If the server returns an undocumented status code and Client.raise_on_unexpected_status is True.
        httpx.TimeoutException: If the request takes longer than Client.timeout.

    Returns:
        Application | Message
    """

    return (
        await asyncio_detailed(
            client=client,
            body=body,
        )
    ).parsed

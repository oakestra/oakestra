from http import HTTPStatus
from typing import Any
from urllib.parse import quote

import httpx

from ... import errors
from ...client import AuthenticatedClient, Client
from ...models.application import Application
from ...models.message import Message
from ...types import UNSET, Response, Unset


def _get_kwargs(
    id: str,
    *,
    application_name: str | Unset = UNSET,
    application_namespace: str | Unset = UNSET,
    user_id: str | Unset = UNSET,
) -> dict[str, Any]:

    params: dict[str, Any] = {}

    params["application_name"] = application_name

    params["application_namespace"] = application_namespace

    params["userId"] = user_id

    params = {k: v for k, v in params.items() if v is not UNSET and v is not None}

    _kwargs: dict[str, Any] = {
        "method": "get",
        "url": "/api/v1/applications/{id}".format(
            id=quote(str(id), safe=""),
        ),
        "params": params,
    }

    return _kwargs


def _parse_response(
    *, client: AuthenticatedClient | Client, response: httpx.Response
) -> Application | Message | None:
    if response.status_code == 200:
        response_200 = Application.from_dict(response.json())

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
) -> Response[Application | Message]:
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
    application_name: str | Unset = UNSET,
    application_namespace: str | Unset = UNSET,
    user_id: str | Unset = UNSET,
) -> Response[Application | Message]:
    """Fetch one application

    Args:
        id (str):  Example: 65f1c0d2a1b2c3d4e5f60718.
        application_name (str | Unset):
        application_namespace (str | Unset):
        user_id (str | Unset):

    Raises:
        errors.UnexpectedStatus: If the server returns an undocumented status code and Client.raise_on_unexpected_status is True.
        httpx.TimeoutException: If the request takes longer than Client.timeout.

    Returns:
        Response[Application | Message]
    """

    kwargs = _get_kwargs(
        id=id,
        application_name=application_name,
        application_namespace=application_namespace,
        user_id=user_id,
    )

    response = client.get_httpx_client().request(
        **kwargs,
    )

    return _build_response(client=client, response=response)


def sync(
    id: str,
    *,
    client: AuthenticatedClient | Client,
    application_name: str | Unset = UNSET,
    application_namespace: str | Unset = UNSET,
    user_id: str | Unset = UNSET,
) -> Application | Message | None:
    """Fetch one application

    Args:
        id (str):  Example: 65f1c0d2a1b2c3d4e5f60718.
        application_name (str | Unset):
        application_namespace (str | Unset):
        user_id (str | Unset):

    Raises:
        errors.UnexpectedStatus: If the server returns an undocumented status code and Client.raise_on_unexpected_status is True.
        httpx.TimeoutException: If the request takes longer than Client.timeout.

    Returns:
        Application | Message
    """

    return sync_detailed(
        id=id,
        client=client,
        application_name=application_name,
        application_namespace=application_namespace,
        user_id=user_id,
    ).parsed


async def asyncio_detailed(
    id: str,
    *,
    client: AuthenticatedClient | Client,
    application_name: str | Unset = UNSET,
    application_namespace: str | Unset = UNSET,
    user_id: str | Unset = UNSET,
) -> Response[Application | Message]:
    """Fetch one application

    Args:
        id (str):  Example: 65f1c0d2a1b2c3d4e5f60718.
        application_name (str | Unset):
        application_namespace (str | Unset):
        user_id (str | Unset):

    Raises:
        errors.UnexpectedStatus: If the server returns an undocumented status code and Client.raise_on_unexpected_status is True.
        httpx.TimeoutException: If the request takes longer than Client.timeout.

    Returns:
        Response[Application | Message]
    """

    kwargs = _get_kwargs(
        id=id,
        application_name=application_name,
        application_namespace=application_namespace,
        user_id=user_id,
    )

    response = await client.get_async_httpx_client().request(**kwargs)

    return _build_response(client=client, response=response)


async def asyncio(
    id: str,
    *,
    client: AuthenticatedClient | Client,
    application_name: str | Unset = UNSET,
    application_namespace: str | Unset = UNSET,
    user_id: str | Unset = UNSET,
) -> Application | Message | None:
    """Fetch one application

    Args:
        id (str):  Example: 65f1c0d2a1b2c3d4e5f60718.
        application_name (str | Unset):
        application_namespace (str | Unset):
        user_id (str | Unset):

    Raises:
        errors.UnexpectedStatus: If the server returns an undocumented status code and Client.raise_on_unexpected_status is True.
        httpx.TimeoutException: If the request takes longer than Client.timeout.

    Returns:
        Application | Message
    """

    return (
        await asyncio_detailed(
            id=id,
            client=client,
            application_name=application_name,
            application_namespace=application_namespace,
            user_id=user_id,
        )
    ).parsed

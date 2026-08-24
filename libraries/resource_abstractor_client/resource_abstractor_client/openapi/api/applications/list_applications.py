from http import HTTPStatus
from typing import Any

import httpx

from ... import errors
from ...client import AuthenticatedClient, Client
from ...models.application import Application
from ...types import UNSET, Response, Unset


def _get_kwargs(
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
        "url": "/api/v1/applications",
        "params": params,
    }

    return _kwargs


def _parse_response(
    *, client: AuthenticatedClient | Client, response: httpx.Response
) -> list[Application] | None:
    if response.status_code == 200:
        response_200 = []
        _response_200 = response.json()
        for response_200_item_data in _response_200:
            response_200_item = Application.from_dict(response_200_item_data)

            response_200.append(response_200_item)

        return response_200

    if client.raise_on_unexpected_status:
        raise errors.UnexpectedStatus(response.status_code, response.content)
    else:
        return None


def _build_response(
    *, client: AuthenticatedClient | Client, response: httpx.Response
) -> Response[list[Application]]:
    return Response(
        status_code=HTTPStatus(response.status_code),
        content=response.content,
        headers=response.headers,
        parsed=_parse_response(client=client, response=response),
    )


def sync_detailed(
    *,
    client: AuthenticatedClient | Client,
    application_name: str | Unset = UNSET,
    application_namespace: str | Unset = UNSET,
    user_id: str | Unset = UNSET,
) -> Response[list[Application]]:
    """List applications

    Args:
        application_name (str | Unset):
        application_namespace (str | Unset):
        user_id (str | Unset):

    Raises:
        errors.UnexpectedStatus: If the server returns an undocumented status code and Client.raise_on_unexpected_status is True.
        httpx.TimeoutException: If the request takes longer than Client.timeout.

    Returns:
        Response[list[Application]]
    """

    kwargs = _get_kwargs(
        application_name=application_name,
        application_namespace=application_namespace,
        user_id=user_id,
    )

    response = client.get_httpx_client().request(
        **kwargs,
    )

    return _build_response(client=client, response=response)


def sync(
    *,
    client: AuthenticatedClient | Client,
    application_name: str | Unset = UNSET,
    application_namespace: str | Unset = UNSET,
    user_id: str | Unset = UNSET,
) -> list[Application] | None:
    """List applications

    Args:
        application_name (str | Unset):
        application_namespace (str | Unset):
        user_id (str | Unset):

    Raises:
        errors.UnexpectedStatus: If the server returns an undocumented status code and Client.raise_on_unexpected_status is True.
        httpx.TimeoutException: If the request takes longer than Client.timeout.

    Returns:
        list[Application]
    """

    return sync_detailed(
        client=client,
        application_name=application_name,
        application_namespace=application_namespace,
        user_id=user_id,
    ).parsed


async def asyncio_detailed(
    *,
    client: AuthenticatedClient | Client,
    application_name: str | Unset = UNSET,
    application_namespace: str | Unset = UNSET,
    user_id: str | Unset = UNSET,
) -> Response[list[Application]]:
    """List applications

    Args:
        application_name (str | Unset):
        application_namespace (str | Unset):
        user_id (str | Unset):

    Raises:
        errors.UnexpectedStatus: If the server returns an undocumented status code and Client.raise_on_unexpected_status is True.
        httpx.TimeoutException: If the request takes longer than Client.timeout.

    Returns:
        Response[list[Application]]
    """

    kwargs = _get_kwargs(
        application_name=application_name,
        application_namespace=application_namespace,
        user_id=user_id,
    )

    response = await client.get_async_httpx_client().request(**kwargs)

    return _build_response(client=client, response=response)


async def asyncio(
    *,
    client: AuthenticatedClient | Client,
    application_name: str | Unset = UNSET,
    application_namespace: str | Unset = UNSET,
    user_id: str | Unset = UNSET,
) -> list[Application] | None:
    """List applications

    Args:
        application_name (str | Unset):
        application_namespace (str | Unset):
        user_id (str | Unset):

    Raises:
        errors.UnexpectedStatus: If the server returns an undocumented status code and Client.raise_on_unexpected_status is True.
        httpx.TimeoutException: If the request takes longer than Client.timeout.

    Returns:
        list[Application]
    """

    return (
        await asyncio_detailed(
            client=client,
            application_name=application_name,
            application_namespace=application_namespace,
            user_id=user_id,
        )
    ).parsed

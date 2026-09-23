from http import HTTPStatus
from typing import Any
from urllib.parse import quote

import httpx

from ... import errors
from ...client import AuthenticatedClient, Client
from ...models.custom_resource_instance import CustomResourceInstance
from ...models.message import Message
from ...types import Response


def _get_kwargs(
    resource: str,
    id: str,
    *,
    body: CustomResourceInstance,
) -> dict[str, Any]:
    headers: dict[str, Any] = {}

    _kwargs: dict[str, Any] = {
        "method": "patch",
        "url": "/api/v1/custom-resources/{resource}/{id}".format(
            resource=quote(str(resource), safe=""),
            id=quote(str(id), safe=""),
        ),
    }

    _kwargs["json"] = body.to_dict()

    headers["Content-Type"] = "application/json"

    _kwargs["headers"] = headers
    return _kwargs


def _parse_response(
    *, client: AuthenticatedClient | Client, response: httpx.Response
) -> CustomResourceInstance | Message | None:
    if response.status_code == 200:
        response_200 = CustomResourceInstance.from_dict(response.json())

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
) -> Response[CustomResourceInstance | Message]:
    return Response(
        status_code=HTTPStatus(response.status_code),
        content=response.content,
        headers=response.headers,
        parsed=_parse_response(client=client, response=response),
    )


def sync_detailed(
    resource: str,
    id: str,
    *,
    client: AuthenticatedClient | Client,
    body: CustomResourceInstance,
) -> Response[CustomResourceInstance | Message]:
    """Update a custom resource instance

     As with creation, the body is validated against the type's stored JSON
    Schema before it is persisted.

    Args:
        resource (str):
        id (str):  Example: 65f1c0d2a1b2c3d4e5f60718.
        body (CustomResourceInstance): One instance of a registered custom resource type. Its
            fields are
            whatever the type's own JSON Schema allows, so nothing beyond `_id` is
            fixed here.

    Raises:
        errors.UnexpectedStatus: If the server returns an undocumented status code and Client.raise_on_unexpected_status is True.
        httpx.TimeoutException: If the request takes longer than Client.timeout.

    Returns:
        Response[CustomResourceInstance | Message]
    """

    kwargs = _get_kwargs(
        resource=resource,
        id=id,
        body=body,
    )

    response = client.get_httpx_client().request(
        **kwargs,
    )

    return _build_response(client=client, response=response)


def sync(
    resource: str,
    id: str,
    *,
    client: AuthenticatedClient | Client,
    body: CustomResourceInstance,
) -> CustomResourceInstance | Message | None:
    """Update a custom resource instance

     As with creation, the body is validated against the type's stored JSON
    Schema before it is persisted.

    Args:
        resource (str):
        id (str):  Example: 65f1c0d2a1b2c3d4e5f60718.
        body (CustomResourceInstance): One instance of a registered custom resource type. Its
            fields are
            whatever the type's own JSON Schema allows, so nothing beyond `_id` is
            fixed here.

    Raises:
        errors.UnexpectedStatus: If the server returns an undocumented status code and Client.raise_on_unexpected_status is True.
        httpx.TimeoutException: If the request takes longer than Client.timeout.

    Returns:
        CustomResourceInstance | Message
    """

    return sync_detailed(
        resource=resource,
        id=id,
        client=client,
        body=body,
    ).parsed


async def asyncio_detailed(
    resource: str,
    id: str,
    *,
    client: AuthenticatedClient | Client,
    body: CustomResourceInstance,
) -> Response[CustomResourceInstance | Message]:
    """Update a custom resource instance

     As with creation, the body is validated against the type's stored JSON
    Schema before it is persisted.

    Args:
        resource (str):
        id (str):  Example: 65f1c0d2a1b2c3d4e5f60718.
        body (CustomResourceInstance): One instance of a registered custom resource type. Its
            fields are
            whatever the type's own JSON Schema allows, so nothing beyond `_id` is
            fixed here.

    Raises:
        errors.UnexpectedStatus: If the server returns an undocumented status code and Client.raise_on_unexpected_status is True.
        httpx.TimeoutException: If the request takes longer than Client.timeout.

    Returns:
        Response[CustomResourceInstance | Message]
    """

    kwargs = _get_kwargs(
        resource=resource,
        id=id,
        body=body,
    )

    response = await client.get_async_httpx_client().request(**kwargs)

    return _build_response(client=client, response=response)


async def asyncio(
    resource: str,
    id: str,
    *,
    client: AuthenticatedClient | Client,
    body: CustomResourceInstance,
) -> CustomResourceInstance | Message | None:
    """Update a custom resource instance

     As with creation, the body is validated against the type's stored JSON
    Schema before it is persisted.

    Args:
        resource (str):
        id (str):  Example: 65f1c0d2a1b2c3d4e5f60718.
        body (CustomResourceInstance): One instance of a registered custom resource type. Its
            fields are
            whatever the type's own JSON Schema allows, so nothing beyond `_id` is
            fixed here.

    Raises:
        errors.UnexpectedStatus: If the server returns an undocumented status code and Client.raise_on_unexpected_status is True.
        httpx.TimeoutException: If the request takes longer than Client.timeout.

    Returns:
        CustomResourceInstance | Message
    """

    return (
        await asyncio_detailed(
            resource=resource,
            id=id,
            client=client,
            body=body,
        )
    ).parsed

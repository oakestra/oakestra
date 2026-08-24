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
    *,
    body: CustomResourceInstance,
) -> dict[str, Any]:
    headers: dict[str, Any] = {}

    _kwargs: dict[str, Any] = {
        "method": "post",
        "url": "/api/v1/custom-resources/{resource}".format(
            resource=quote(str(resource), safe=""),
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
    *,
    client: AuthenticatedClient | Client,
    body: CustomResourceInstance,
) -> Response[CustomResourceInstance | Message]:
    """Create an instance of a custom resource type

     The body is validated against the JSON Schema stored in the type's
    definition before it is persisted; a schema violation is reported as
    400 with the validator's own message.

    Args:
        resource (str):
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
        body=body,
    )

    response = client.get_httpx_client().request(
        **kwargs,
    )

    return _build_response(client=client, response=response)


def sync(
    resource: str,
    *,
    client: AuthenticatedClient | Client,
    body: CustomResourceInstance,
) -> CustomResourceInstance | Message | None:
    """Create an instance of a custom resource type

     The body is validated against the JSON Schema stored in the type's
    definition before it is persisted; a schema violation is reported as
    400 with the validator's own message.

    Args:
        resource (str):
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
        client=client,
        body=body,
    ).parsed


async def asyncio_detailed(
    resource: str,
    *,
    client: AuthenticatedClient | Client,
    body: CustomResourceInstance,
) -> Response[CustomResourceInstance | Message]:
    """Create an instance of a custom resource type

     The body is validated against the JSON Schema stored in the type's
    definition before it is persisted; a schema violation is reported as
    400 with the validator's own message.

    Args:
        resource (str):
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
        body=body,
    )

    response = await client.get_async_httpx_client().request(**kwargs)

    return _build_response(client=client, response=response)


async def asyncio(
    resource: str,
    *,
    client: AuthenticatedClient | Client,
    body: CustomResourceInstance,
) -> CustomResourceInstance | Message | None:
    """Create an instance of a custom resource type

     The body is validated against the JSON Schema stored in the type's
    definition before it is persisted; a schema violation is reported as
    400 with the validator's own message.

    Args:
        resource (str):
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
            client=client,
            body=body,
        )
    ).parsed

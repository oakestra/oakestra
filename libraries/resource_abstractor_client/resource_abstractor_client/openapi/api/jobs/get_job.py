from http import HTTPStatus
from typing import Any
from urllib.parse import quote

import httpx

from ... import errors
from ...client import AuthenticatedClient, Client
from ...models.job import Job
from ...models.message import Message
from ...models.validation_error import ValidationError
from ...types import UNSET, Response, Unset


def _get_kwargs(
    job_id: str,
    *,
    instance_number: str | Unset = UNSET,
) -> dict[str, Any]:

    params: dict[str, Any] = {}

    params["instance_number"] = instance_number

    params = {k: v for k, v in params.items() if v is not UNSET and v is not None}

    _kwargs: dict[str, Any] = {
        "method": "get",
        "url": "/api/v1/jobs/{job_id}".format(
            job_id=quote(str(job_id), safe=""),
        ),
        "params": params,
    }

    return _kwargs


def _parse_response(
    *, client: AuthenticatedClient | Client, response: httpx.Response
) -> Job | Message | ValidationError | None:
    if response.status_code == 200:
        response_200 = Job.from_dict(response.json())

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
) -> Response[Job | Message | ValidationError]:
    return Response(
        status_code=HTTPStatus(response.status_code),
        content=response.content,
        headers=response.headers,
        parsed=_parse_response(client=client, response=response),
    )


def sync_detailed(
    job_id: str,
    *,
    client: AuthenticatedClient | Client,
    instance_number: str | Unset = UNSET,
) -> Response[Job | Message | ValidationError]:
    """Fetch one job

    Args:
        job_id (str):  Example: 65f1c0d2a1b2c3d4e5f60718.
        instance_number (str | Unset):

    Raises:
        errors.UnexpectedStatus: If the server returns an undocumented status code and Client.raise_on_unexpected_status is True.
        httpx.TimeoutException: If the request takes longer than Client.timeout.

    Returns:
        Response[Job | Message | ValidationError]
    """

    kwargs = _get_kwargs(
        job_id=job_id,
        instance_number=instance_number,
    )

    response = client.get_httpx_client().request(
        **kwargs,
    )

    return _build_response(client=client, response=response)


def sync(
    job_id: str,
    *,
    client: AuthenticatedClient | Client,
    instance_number: str | Unset = UNSET,
) -> Job | Message | ValidationError | None:
    """Fetch one job

    Args:
        job_id (str):  Example: 65f1c0d2a1b2c3d4e5f60718.
        instance_number (str | Unset):

    Raises:
        errors.UnexpectedStatus: If the server returns an undocumented status code and Client.raise_on_unexpected_status is True.
        httpx.TimeoutException: If the request takes longer than Client.timeout.

    Returns:
        Job | Message | ValidationError
    """

    return sync_detailed(
        job_id=job_id,
        client=client,
        instance_number=instance_number,
    ).parsed


async def asyncio_detailed(
    job_id: str,
    *,
    client: AuthenticatedClient | Client,
    instance_number: str | Unset = UNSET,
) -> Response[Job | Message | ValidationError]:
    """Fetch one job

    Args:
        job_id (str):  Example: 65f1c0d2a1b2c3d4e5f60718.
        instance_number (str | Unset):

    Raises:
        errors.UnexpectedStatus: If the server returns an undocumented status code and Client.raise_on_unexpected_status is True.
        httpx.TimeoutException: If the request takes longer than Client.timeout.

    Returns:
        Response[Job | Message | ValidationError]
    """

    kwargs = _get_kwargs(
        job_id=job_id,
        instance_number=instance_number,
    )

    response = await client.get_async_httpx_client().request(**kwargs)

    return _build_response(client=client, response=response)


async def asyncio(
    job_id: str,
    *,
    client: AuthenticatedClient | Client,
    instance_number: str | Unset = UNSET,
) -> Job | Message | ValidationError | None:
    """Fetch one job

    Args:
        job_id (str):  Example: 65f1c0d2a1b2c3d4e5f60718.
        instance_number (str | Unset):

    Raises:
        errors.UnexpectedStatus: If the server returns an undocumented status code and Client.raise_on_unexpected_status is True.
        httpx.TimeoutException: If the request takes longer than Client.timeout.

    Returns:
        Job | Message | ValidationError
    """

    return (
        await asyncio_detailed(
            job_id=job_id,
            client=client,
            instance_number=instance_number,
        )
    ).parsed

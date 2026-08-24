from http import HTTPStatus
from typing import Any
from urllib.parse import quote

import httpx

from ... import errors
from ...client import AuthenticatedClient, Client
from ...models.job import Job
from ...models.message import Message
from ...types import Response


def _get_kwargs(
    job_id: str,
    instance_id: int,
) -> dict[str, Any]:

    _kwargs: dict[str, Any] = {
        "method": "get",
        "url": "/api/v1/jobs/{job_id}/{instance_id}".format(
            job_id=quote(str(job_id), safe=""),
            instance_id=quote(str(instance_id), safe=""),
        ),
    }

    return _kwargs


def _parse_response(
    *, client: AuthenticatedClient | Client, response: httpx.Response
) -> Job | Message | None:
    if response.status_code == 200:
        response_200 = Job.from_dict(response.json())

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
) -> Response[Job | Message]:
    return Response(
        status_code=HTTPStatus(response.status_code),
        content=response.content,
        headers=response.headers,
        parsed=_parse_response(client=client, response=response),
    )


def sync_detailed(
    job_id: str,
    instance_id: int,
    *,
    client: AuthenticatedClient | Client,
) -> Response[Job | Message]:
    """Fetch one job instance

     Returns the job, with `instance_list` filtered down to just the
    matching instance - not a bare instance object, the same shape
    `put`/`patch`/`delete` on this path return.

    Args:
        job_id (str):  Example: 65f1c0d2a1b2c3d4e5f60718.
        instance_id (int):

    Raises:
        errors.UnexpectedStatus: If the server returns an undocumented status code and Client.raise_on_unexpected_status is True.
        httpx.TimeoutException: If the request takes longer than Client.timeout.

    Returns:
        Response[Job | Message]
    """

    kwargs = _get_kwargs(
        job_id=job_id,
        instance_id=instance_id,
    )

    response = client.get_httpx_client().request(
        **kwargs,
    )

    return _build_response(client=client, response=response)


def sync(
    job_id: str,
    instance_id: int,
    *,
    client: AuthenticatedClient | Client,
) -> Job | Message | None:
    """Fetch one job instance

     Returns the job, with `instance_list` filtered down to just the
    matching instance - not a bare instance object, the same shape
    `put`/`patch`/`delete` on this path return.

    Args:
        job_id (str):  Example: 65f1c0d2a1b2c3d4e5f60718.
        instance_id (int):

    Raises:
        errors.UnexpectedStatus: If the server returns an undocumented status code and Client.raise_on_unexpected_status is True.
        httpx.TimeoutException: If the request takes longer than Client.timeout.

    Returns:
        Job | Message
    """

    return sync_detailed(
        job_id=job_id,
        instance_id=instance_id,
        client=client,
    ).parsed


async def asyncio_detailed(
    job_id: str,
    instance_id: int,
    *,
    client: AuthenticatedClient | Client,
) -> Response[Job | Message]:
    """Fetch one job instance

     Returns the job, with `instance_list` filtered down to just the
    matching instance - not a bare instance object, the same shape
    `put`/`patch`/`delete` on this path return.

    Args:
        job_id (str):  Example: 65f1c0d2a1b2c3d4e5f60718.
        instance_id (int):

    Raises:
        errors.UnexpectedStatus: If the server returns an undocumented status code and Client.raise_on_unexpected_status is True.
        httpx.TimeoutException: If the request takes longer than Client.timeout.

    Returns:
        Response[Job | Message]
    """

    kwargs = _get_kwargs(
        job_id=job_id,
        instance_id=instance_id,
    )

    response = await client.get_async_httpx_client().request(**kwargs)

    return _build_response(client=client, response=response)


async def asyncio(
    job_id: str,
    instance_id: int,
    *,
    client: AuthenticatedClient | Client,
) -> Job | Message | None:
    """Fetch one job instance

     Returns the job, with `instance_list` filtered down to just the
    matching instance - not a bare instance object, the same shape
    `put`/`patch`/`delete` on this path return.

    Args:
        job_id (str):  Example: 65f1c0d2a1b2c3d4e5f60718.
        instance_id (int):

    Raises:
        errors.UnexpectedStatus: If the server returns an undocumented status code and Client.raise_on_unexpected_status is True.
        httpx.TimeoutException: If the request takes longer than Client.timeout.

    Returns:
        Job | Message
    """

    return (
        await asyncio_detailed(
            job_id=job_id,
            instance_id=instance_id,
            client=client,
        )
    ).parsed

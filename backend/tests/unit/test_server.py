import asyncio

import pytest

from app.server import supervise_services


@pytest.mark.asyncio
async def test_supervisor_stops_remaining_services_when_one_exits() -> None:
    started = asyncio.Event()
    stopped = asyncio.Event()

    async def background_service() -> None:
        started.set()
        try:
            await asyncio.Future()
        finally:
            stopped.set()

    async def completed_service() -> None:
        await started.wait()

    await supervise_services(background_service, completed_service)

    assert stopped.is_set()


@pytest.mark.asyncio
async def test_supervisor_propagates_service_failure_after_stopping_peers() -> None:
    started = asyncio.Event()
    stopped = asyncio.Event()

    async def background_service() -> None:
        started.set()
        try:
            await asyncio.Future()
        finally:
            stopped.set()

    async def failed_service() -> None:
        await started.wait()
        raise RuntimeError("scheduler failed")

    with pytest.raises(RuntimeError, match="scheduler failed"):
        await supervise_services(background_service, failed_service)

    assert stopped.is_set()

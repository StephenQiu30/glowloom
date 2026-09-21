import asyncio
from typing import Any, cast
from unittest.mock import AsyncMock, Mock

import pytest
from temporalio.client import Client

from app.core.config import Settings, WorkerSettings
from app.core.container import AgentRuntime
from app.creation.repository import Repository
from app.skills.runtime import SkillRuntime


def runtime_fixture(monkeypatch: pytest.MonkeyPatch) -> AgentRuntime:
    settings = Settings("postgresql://test/test", "s" * 32, "localhost:7233", "default", "q", False)
    monkeypatch.setattr(WorkerSettings, "from_environment", Mock(return_value=Mock(base=settings)))
    return AgentRuntime(
        repository=cast(Repository, AsyncMock()),
        secret=settings.secret,
        task_queue=settings.task_queue,
        settings=settings,
        temporal=cast(Client, Mock()),
        skill_runtime=cast(SkillRuntime, Mock()),
        request_shutdown=Mock(),
    )


@pytest.mark.parametrize("component", ["worker", "dispatcher"])
async def test_background_exit_revokes_readiness_and_requests_shutdown(
    monkeypatch: pytest.MonkeyPatch, component: str
) -> None:
    runtime = runtime_fixture(monkeypatch)
    fail = asyncio.Event()

    async def worker(stop: asyncio.Event, ready: asyncio.Event, **_: Any) -> None:
        ready.set()
        await (fail if component == "worker" else stop).wait()
        if component == "worker":
            raise RuntimeError("private diagnostic must not escape lifecycle")

    async def dispatch(_: Any) -> None:
        if component == "dispatcher":
            await fail.wait()
            return  # Unexpected clean exits are failures too.
        await asyncio.Event().wait()

    monkeypatch.setattr("app.core.container.run_worker", worker)
    monkeypatch.setattr("app.core.container.Dispatcher.run", dispatch)
    await runtime.start()
    assert runtime.is_ready
    fail.set()
    await asyncio.sleep(0)
    await asyncio.sleep(0)
    assert not runtime.is_ready
    cast(Mock, runtime.request_shutdown).assert_called_once()
    await runtime.stop()
    assert runtime.worker_task is None and runtime.dispatch_task is None


async def test_startup_failure_cleans_dispatcher_and_preserves_original_error(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    from fastapi import FastAPI

    from app.core.lifespan import lifespan

    runtime = runtime_fixture(monkeypatch)
    dispatch_stopped = asyncio.Event()

    async def dispatch(_: Any) -> None:
        try:
            await asyncio.Event().wait()
        finally:
            dispatch_stopped.set()

    async def worker(*_: Any, **__: Any) -> None:
        raise RuntimeError("worker startup failed")

    monkeypatch.setattr("app.core.container.run_worker", worker)
    monkeypatch.setattr("app.core.container.Dispatcher.run", dispatch)
    app = FastAPI()
    app.state.runtime = runtime
    with pytest.raises(RuntimeError, match="worker startup failed"):
        async with lifespan(app):
            pytest.fail("startup must not finish")
    assert dispatch_stopped.is_set()
    assert runtime.worker_task is None and runtime.dispatch_task is None
    cast(Mock, runtime.request_shutdown).assert_not_called()


async def test_orderly_shutdown_stops_dispatch_before_worker_and_does_not_signal(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    runtime = runtime_fixture(monkeypatch)
    dispatch_stopped = asyncio.Event()

    async def dispatch(_: Any) -> None:
        try:
            await asyncio.Event().wait()
        finally:
            dispatch_stopped.set()

    async def worker(stop: asyncio.Event, ready: asyncio.Event, **_: Any) -> None:
        ready.set()
        await stop.wait()
        assert dispatch_stopped.is_set()

    monkeypatch.setattr("app.core.container.run_worker", worker)
    monkeypatch.setattr("app.core.container.Dispatcher.run", dispatch)
    await runtime.start()
    await runtime.stop()
    assert not runtime.is_ready
    cast(Mock, runtime.request_shutdown).assert_not_called()

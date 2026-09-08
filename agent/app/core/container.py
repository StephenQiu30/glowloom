"""Explicit dependencies owned by the Agent application process."""

from __future__ import annotations

import asyncio
from collections.abc import Callable
from contextlib import suppress
from dataclasses import dataclass, field

from temporalio.client import Client

from app.core.config import Settings, WorkerSettings
from app.creation.dispatcher import Dispatcher
from app.creation.repository import Repository
from app.creation.temporal import TemporalStarter
from app.creation.worker import run_worker
from app.skills.catalog import SkillCatalog
from app.skills.runtime import SkillRuntime


@dataclass
class AgentRuntime:
    """Runtime resources shared by HTTP routes and the Temporal worker."""

    repository: Repository
    secret: str
    task_queue: str
    settings: Settings | None = None
    temporal: Client | None = None
    skill_runtime: SkillRuntime = field(default_factory=lambda: SkillRuntime(SkillCatalog()))
    runtime_ready: Callable[[], bool] | None = None
    dispatcher: Dispatcher | None = field(default=None, init=False)
    dispatch_task: asyncio.Task[None] | None = field(default=None, init=False)
    worker_task: asyncio.Task[None] | None = field(default=None, init=False)
    worker_stop: asyncio.Event | None = field(default=None, init=False)

    @property
    def skill_catalog(self) -> SkillCatalog:
        return self.skill_runtime.catalog

    @property
    def worker_enabled(self) -> bool:
        return self.settings is not None

    async def start(self) -> None:
        """Start durable resources owned by the production application lifespan."""

        await self.repository.ready()
        self.skill_runtime.verify_all()
        if not self.worker_enabled:
            return
        assert self.settings is not None
        if self.temporal is None:
            self.temporal = await Client.connect(
                self.settings.temporal_address,
                namespace=self.settings.temporal_namespace,
                tls=self.settings.temporal_tls,
            )
        worker_settings = WorkerSettings.from_environment()
        self.dispatcher = Dispatcher(self.repository, TemporalStarter(self.temporal))
        self.dispatch_task = asyncio.create_task(
            self.dispatcher.run(), name="creation-start-dispatcher"
        )
        self.worker_stop = asyncio.Event()
        worker_ready = asyncio.Event()
        self.worker_task = asyncio.create_task(
            run_worker(
                self.worker_stop,
                worker_ready,
                settings=worker_settings,
                repository=self.repository,
                client=self.temporal,
            ),
            name="agent-temporal-worker",
        )
        ready_wait = asyncio.create_task(worker_ready.wait(), name="agent-worker-ready")
        done, _ = await asyncio.wait(
            {ready_wait, self.worker_task}, timeout=30, return_when=asyncio.FIRST_COMPLETED
        )
        ready_wait.cancel()
        with suppress(asyncio.CancelledError):
            await ready_wait
        if self.worker_task in done:
            self.worker_task.result()
        if not done:
            raise TimeoutError("Agent Temporal Worker did not become ready")

    async def stop(self) -> None:
        """Stop owned background work without leaving an unbounded task."""

        if self.worker_stop is not None:
            self.worker_stop.set()
        if self.worker_task is not None:
            try:
                await asyncio.wait_for(asyncio.shield(self.worker_task), timeout=30)
            except TimeoutError:
                self.worker_task.cancel()
                with suppress(asyncio.CancelledError):
                    await self.worker_task
            finally:
                self.worker_task = None
                self.worker_stop = None
        if self.dispatch_task is not None:
            self.dispatch_task.cancel()
            with suppress(asyncio.CancelledError):
                await self.dispatch_task
            self.dispatch_task = None

"""Run the trusted worker: python -m app.creation.worker."""

import asyncio
import signal
from datetime import timedelta

import httpx
from temporalio.client import Client
from temporalio.worker import Worker

from app.creation.activities import CreationActivities
from app.creation.config import WorkerSettings
from app.creation.execution import ExecutionStore
from app.creation.platform import HarnessClient, PlatformClient
from app.creation.repository import Repository
from app.creation.workflow import TextStoryboardWorkflow


async def run_worker(stop: asyncio.Event | None = None, ready: asyncio.Event | None = None) -> None:
    if stop is None:
        stop = asyncio.Event()
        loop = asyncio.get_running_loop()
        for signum in (signal.SIGTERM, signal.SIGINT):
            loop.add_signal_handler(signum, stop.set)
    assert stop is not None
    settings = WorkerSettings.from_environment()
    repository = Repository(settings.base.database_url)
    await repository.ready()
    client = await Client.connect(
        settings.base.temporal_address,
        namespace=settings.base.temporal_namespace,
        tls=settings.base.temporal_tls,
    )
    async with httpx.AsyncClient(follow_redirects=False, trust_env=False) as http:
        activities = CreationActivities(
            ExecutionStore(repository),
            PlatformClient(http, settings.platform_url, settings.base.secret),
            HarnessClient(http, settings.harness_url, settings.harness_secret),
            settings.release_hash,
            settings.call_limit,
            settings.invocation_timeout_seconds,
        )
        worker = Worker(
            client,
            task_queue=settings.base.task_queue,
            workflows=[TextStoryboardWorkflow],
            activities=[activities.invoke, activities.gate, activities.progress],
            max_concurrent_activities=4,
            graceful_shutdown_timeout=timedelta(seconds=settings.invocation_timeout_seconds + 20),
        )
        async with worker:
            if ready is not None:
                ready.set()
            await stop.wait()


if __name__ == "__main__":
    asyncio.run(run_worker())

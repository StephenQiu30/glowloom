"""Temporal registration; resources are injected by the application container."""

import asyncio
from datetime import timedelta

import httpx
from temporalio.client import Client
from temporalio.worker import Worker

from app.core.config import WorkerSettings
from app.creation.activities import CreationActivities, TextInvoker
from app.creation.execution import ExecutionStore
from app.creation.platform import PlatformClient
from app.creation.repository import Repository
from app.creation.workflow import TextStoryboardWorkflow


async def run_worker(
    stop: asyncio.Event,
    ready: asyncio.Event,
    *,
    settings: WorkerSettings,
    repository: Repository,
    client: Client,
    harness: TextInvoker,
) -> None:
    async with httpx.AsyncClient(follow_redirects=False, trust_env=False) as http:
        activities = CreationActivities(
            ExecutionStore(repository),
            PlatformClient(http, settings.platform_url, settings.base.secret),
            harness,
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
            ready.set()
            await stop.wait()

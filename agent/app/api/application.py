"""Canonical FastAPI application factory for the single Agent service."""

from __future__ import annotations

import asyncio
from collections.abc import AsyncGenerator
from contextlib import asynccontextmanager, suppress

from fastapi import FastAPI

from app.api.router import build_router
from app.candidate_runtime.api import verify_bundles
from app.candidate_runtime.readiness import candidate_runtime_ready
from app.creation.api import create_configured_app
from app.creation.worker import run_worker


def create_agent_app() -> FastAPI:
    """Build the trusted Agent application and its restricted capability routes."""

    app = create_configured_app(runtime_ready=candidate_runtime_ready)
    app.include_router(build_router())
    creation_lifespan = app.router.lifespan_context

    @asynccontextmanager
    async def lifespan(application: FastAPI) -> AsyncGenerator[None, None]:
        async with creation_lifespan(application):
            verify_bundles()
            stop = asyncio.Event()
            worker_ready = asyncio.Event()
            worker_task = asyncio.create_task(
                run_worker(stop, worker_ready), name="agent-temporal-worker"
            )
            try:
                ready_wait = asyncio.create_task(worker_ready.wait(), name="agent-worker-ready")
                done, _ = await asyncio.wait(
                    {ready_wait, worker_task}, timeout=30, return_when=asyncio.FIRST_COMPLETED
                )
                ready_wait.cancel()
                with suppress(asyncio.CancelledError):
                    await ready_wait
                if worker_task in done:
                    worker_task.result()
                if not done:
                    raise TimeoutError("Agent Temporal Worker did not become ready")
                yield
            finally:
                stop.set()
                try:
                    await asyncio.wait_for(asyncio.shield(worker_task), timeout=30)
                except TimeoutError:
                    worker_task.cancel()
                    with suppress(asyncio.CancelledError):
                        await worker_task

    app.router.lifespan_context = lifespan
    return app

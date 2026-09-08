"""The single FastAPI lifespan for the Agent process."""

from __future__ import annotations

from collections.abc import AsyncGenerator
from contextlib import asynccontextmanager

from fastapi import FastAPI

from app.core.container import AgentRuntime


@asynccontextmanager
async def lifespan(app: FastAPI) -> AsyncGenerator[None, None]:
    """Own startup and shutdown for the API, dispatcher and Temporal worker."""

    runtime: AgentRuntime = app.state.runtime
    try:
        await runtime.start()
    except BaseException:
        await runtime.stop()
        raise
    try:
        yield
    finally:
        await runtime.stop()

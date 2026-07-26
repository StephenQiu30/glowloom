from __future__ import annotations

from dataclasses import dataclass
from typing import Protocol
from uuid import UUID

from schemas.jobs import JobPayload


class UnknownJobHandler(LookupError):
    pass


@dataclass(frozen=True, slots=True)
class JobContext:
    job_id: UUID
    owner: str
    payload: JobPayload


class JobHandler(Protocol):
    async def handle(self, context: JobContext) -> None: ...


class JobHandlerRegistry:
    def __init__(self) -> None:
        self._handlers: dict[tuple[str, str, str], JobHandler] = {}

    def register(
        self,
        *,
        task_type: str,
        release_version: str,
        handler_version: str,
        handler: JobHandler,
    ) -> None:
        key = (task_type, release_version, handler_version)
        if key in self._handlers:
            raise ValueError("job handler is already registered")
        self._handlers[key] = handler

    async def dispatch(self, task_type: str, context: JobContext) -> None:
        key = (
            task_type,
            context.payload.release_version,
            context.payload.handler_version,
        )
        handler = self._handlers.get(key)
        if handler is None:
            raise UnknownJobHandler(f"no compatible handler for {key!r}")
        await handler.handle(context)

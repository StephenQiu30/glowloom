from __future__ import annotations

from dataclasses import dataclass
from typing import Protocol
from uuid import UUID


class InvalidJobPayload(ValueError):
    pass


class UnknownJobHandler(LookupError):
    pass


@dataclass(frozen=True, slots=True)
class JobPayload:
    release_version: str
    handler_version: str
    task_id: UUID
    snapshot_id: UUID

    @classmethod
    def parse(cls, value: dict[str, object]) -> JobPayload:
        if set(value) != {
            "release_version",
            "handler_version",
            "task_id",
            "snapshot_id",
        }:
            raise InvalidJobPayload("TaskJob payload fields do not match the contract")
        try:
            release_version = str(value["release_version"])
            handler_version = str(value["handler_version"])
            task_id = UUID(str(value["task_id"]))
            snapshot_id = UUID(str(value["snapshot_id"]))
        except (KeyError, TypeError, ValueError) as error:
            raise InvalidJobPayload("TaskJob payload values are invalid") from error
        if not release_version.strip() or not handler_version.strip():
            raise InvalidJobPayload("TaskJob versions must be non-empty")
        return cls(
            release_version=release_version,
            handler_version=handler_version,
            task_id=task_id,
            snapshot_id=snapshot_id,
        )


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

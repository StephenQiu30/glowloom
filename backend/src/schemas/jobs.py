from __future__ import annotations

from dataclasses import dataclass
from uuid import UUID


class InvalidJobPayload(ValueError):
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

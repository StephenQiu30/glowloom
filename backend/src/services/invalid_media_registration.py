from __future__ import annotations

import hashlib
import re

import asyncpg  # type: ignore[import-untyped]

from db.pool import DatabasePool
from integrations.object_storage import ObjectStore
from repositories.media import MediaRegistrationRow, MediaRepository
from schemas.media_registration import (
    MediaRegistrationCommand,
    MediaRegistrationSnapshot,
)
from services.media_errors import MediaRegistrationConflict, MediaTaskMismatch

INPUT_HASH = re.compile(r"^[0-9a-f]{64}$")
CAPABILITY_KIND = {"image": "image", "video": "video", "tts": "audio"}


class InvalidMediaRegistrar:
    def __init__(self, database: DatabasePool, object_store: ObjectStore) -> None:
        self._database = database
        self._object_store = object_store
        self._repository = MediaRepository()

    async def register(
        self, command: MediaRegistrationCommand, *, reason: str
    ) -> MediaRegistrationSnapshot:
        if INPUT_HASH.fullmatch(command.input_hash) is None:
            raise ValueError("input hash must be 64 lowercase hexadecimal characters")
        if not reason.strip():
            raise ValueError("invalid media reason is required")
        await self._assert_task(command)
        location = self._object_store.invalid_location(
            command.episode_id, command.attempt_id, command.output_slot
        )
        sha256 = hashlib.sha256(command.data).hexdigest()
        async with self._database.transaction() as connection:
            await self._assert_task(command, connection=connection, for_update=True)
            existing = await self._repository.find_by_attempt_slot(
                connection, attempt_id=command.attempt_id, output_slot=command.output_slot
            )
            if existing is not None:
                self._assert_replay(existing, command, location.object_key, sha256, reason)
                return self._snapshot(existing)
            row = await self._repository.insert_finalized(
                connection,
                episode_id=command.episode_id,
                task_id=command.task_id,
                attempt_id=command.attempt_id,
                output_slot=command.output_slot,
                usage_type=command.usage_type,
                usage_id=command.usage_id,
                input_version_id=command.input_version_id,
                input_hash=command.input_hash,
                media_kind=command.media_kind,
                bucket=location.bucket,
                object_key=location.object_key,
                content_type=command.content_type,
                byte_size=len(command.data),
                sha256=sha256,
                width=None,
                height=None,
                duration_ticks=None,
                timebase=None,
                probe_summary=None,
                media_status="invalid",
                candidate_status="blocked",
                blocked_reason=reason,
            )
            return self._snapshot(row)

    async def _assert_task(
        self,
        command: MediaRegistrationCommand,
        *,
        connection: asyncpg.Connection[asyncpg.Record] | None = None,
        for_update: bool = False,
    ) -> None:
        if connection is None:
            async with self._database.transaction() as owned:
                context = await self._repository.task_context(
                    owned,
                    task_id=command.task_id,
                    attempt_id=command.attempt_id,
                    for_update=for_update,
                )
        else:
            context = await self._repository.task_context(
                connection,
                task_id=command.task_id,
                attempt_id=command.attempt_id,
                for_update=for_update,
            )
        expected = CAPABILITY_KIND.get(context[1]) if context else None
        if context is None or context[0] != command.episode_id or expected != command.media_kind:
            raise MediaTaskMismatch("attempt does not belong to the requested media task")

    @staticmethod
    def _assert_replay(
        existing: MediaRegistrationRow,
        command: MediaRegistrationCommand,
        object_key: str,
        sha256: str,
        reason: str,
    ) -> None:
        expected = (
            command.episode_id, command.task_id, command.attempt_id, command.output_slot,
            command.usage_type, command.usage_id, command.input_version_id,
            command.input_hash, command.media_kind, object_key, command.content_type,
            len(command.data), sha256, "invalid", "blocked", reason,
        )
        actual = (
            existing.episode_id, existing.task_id, existing.attempt_id,
            existing.output_slot, existing.usage_type, existing.usage_id,
            existing.input_version_id, existing.input_hash, existing.media_kind,
            existing.object_key, existing.content_type, existing.byte_size,
            existing.sha256, existing.media_status, existing.candidate_status,
            existing.blocked_reason,
        )
        if actual != expected:
            raise MediaRegistrationConflict("invalid media replay changed immutable facts")

    @staticmethod
    def _snapshot(row: MediaRegistrationRow) -> MediaRegistrationSnapshot:
        return MediaRegistrationSnapshot(
            row.media_object_id, row.media_version_id, row.candidate_id,
            row.media_status, row.candidate_status, row.bucket, row.object_key, row.sha256,
        )

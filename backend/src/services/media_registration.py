from __future__ import annotations

import re
from dataclasses import dataclass
from typing import Literal, Protocol
from uuid import UUID

import asyncpg  # type: ignore[import-untyped]

from db.pool import DatabasePool
from integrations.object_storage import StoredObject
from repositories.media import MediaRegistrationRow, MediaRepository
from services.media_validation import MediaKind, MediaValidationService, ValidatedMedia

INPUT_HASH = re.compile(r"^[0-9a-f]{64}$")
UsageType = Literal["asset_image", "shot_image", "shot_video", "speech_audio"]


class MediaRegistrationConflict(RuntimeError):
    pass


class MediaTaskMismatch(ValueError):
    pass


class ObjectStorePort(Protocol):
    async def put(
        self,
        *,
        episode_id: UUID,
        attempt_id: UUID,
        output_slot: str,
        content_type: str,
        data: bytes,
    ) -> StoredObject: ...


@dataclass(frozen=True, slots=True)
class MediaRegistrationCommand:
    episode_id: UUID
    task_id: UUID
    attempt_id: UUID
    output_slot: str
    usage_type: UsageType
    usage_id: UUID
    input_version_id: UUID
    input_hash: str
    media_kind: MediaKind
    content_type: str
    data: bytes
    target_duration_ticks: int | None = None

@dataclass(frozen=True, slots=True)
class MediaRegistrationSnapshot:
    media_object_id: UUID
    media_version_id: UUID
    candidate_id: UUID
    media_status: str
    candidate_status: str
    bucket: str
    object_key: str
    sha256: str

class MediaRegistrationService:
    _CAPABILITY_KIND = {"image": "image", "video": "video", "tts": "audio"}
    def __init__(
        self,
        database: DatabasePool,
        validator: MediaValidationService,
        object_store: ObjectStorePort,
    ) -> None:
        self._database = database
        self._validator = validator
        self._object_store = object_store
        self._repository = MediaRepository()

    async def find_registered(
        self, attempt_id: UUID, output_slot: str
    ) -> MediaRegistrationSnapshot | None:
        async with self._database.transaction() as connection:
            row = await self._repository.find_by_attempt_slot(
                connection, attempt_id=attempt_id, output_slot=output_slot
            )
        return self._snapshot(row) if row is not None else None

    async def register(self, command: MediaRegistrationCommand) -> MediaRegistrationSnapshot:
        if INPUT_HASH.fullmatch(command.input_hash) is None:
            raise ValueError("input hash must be 64 lowercase hexadecimal characters")
        await self._assert_task(command, for_update=False)
        validated = await self._validator.validate(
            command.media_kind,
            command.content_type,
            command.data,
            target_duration_ticks=command.target_duration_ticks,
        )
        stored = await self._object_store.put(
            episode_id=command.episode_id,
            attempt_id=command.attempt_id,
            output_slot=command.output_slot,
            content_type=command.content_type,
            data=command.data,
        )
        if stored.sha256 != validated.sha256 or stored.byte_size != validated.byte_size:
            raise MediaRegistrationConflict("stored object facts differ from validated media")
        probe = validated.probe_summary.model_dump(
            mode="json", exclude_none=False, exclude_defaults=False
        )
        async with self._database.transaction() as connection:
            await self._assert_task(command, for_update=True, connection=connection)
            existing = await self._repository.find_by_attempt_slot(
                connection,
                attempt_id=command.attempt_id,
                output_slot=command.output_slot,
            )
            if existing is not None:
                self._assert_replay(existing, command, stored, validated, probe)
                return self._snapshot(existing)
            registered = await self._repository.insert_ready(
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
                bucket=stored.bucket,
                object_key=stored.object_key,
                content_type=stored.content_type,
                byte_size=stored.byte_size,
                sha256=stored.sha256,
                width=validated.width,
                height=validated.height,
                duration_ticks=validated.duration_ticks,
                timebase=validated.timebase,
                probe_summary=probe,
            )
            return self._snapshot(registered)

    async def _assert_task(
        self,
        command: MediaRegistrationCommand,
        *,
        for_update: bool,
        connection: asyncpg.Connection[asyncpg.Record] | None = None,
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
        expected = self._CAPABILITY_KIND.get(context[1]) if context else None
        if context is None or context[0] != command.episode_id or expected != command.media_kind:
            raise MediaTaskMismatch("attempt does not belong to the requested media task")

    @staticmethod
    def _assert_replay(
        existing: MediaRegistrationRow,
        command: MediaRegistrationCommand,
        stored: StoredObject,
        validated: ValidatedMedia,
        probe: dict[str, object],
    ) -> None:
        expected = (
            command.episode_id, command.task_id, command.attempt_id, command.output_slot,
            command.usage_type, command.usage_id, command.input_version_id,
            command.input_hash, command.media_kind, stored.bucket, stored.object_key,
            stored.content_type, stored.byte_size, stored.sha256, validated.width,
            validated.height, validated.duration_ticks, validated.timebase, probe,
        )
        actual = (
            existing.episode_id, existing.task_id, existing.attempt_id,
            existing.output_slot, existing.usage_type, existing.usage_id,
            existing.input_version_id, existing.input_hash, existing.media_kind,
            existing.bucket, existing.object_key, existing.content_type,
            existing.byte_size, existing.sha256, existing.width, existing.height,
            existing.duration_ticks, existing.timebase, dict(existing.probe_summary),
        )
        if actual != expected:
            raise MediaRegistrationConflict("attempt output replay changed immutable facts")

    @staticmethod
    def _snapshot(row: MediaRegistrationRow) -> MediaRegistrationSnapshot:
        return MediaRegistrationSnapshot(
            row.media_object_id, row.media_version_id, row.candidate_id,
            row.media_status, row.candidate_status, row.bucket, row.object_key, row.sha256,
        )

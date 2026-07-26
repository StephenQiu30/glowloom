from __future__ import annotations

from dataclasses import dataclass
from typing import cast
from uuid import UUID

import asyncpg  # type: ignore[import-untyped]

from db.pool import DatabasePool
from integrations.ai.profiles import Capability
from integrations.ai.registry import AiModelRegistry, create_mvp_registry
from integrations.ai.voices import VoiceCatalog, create_mvp_voice_catalog
from schemas.media_registration import UsageType
from schemas.story_content import VoiceId, canonical_content_hash
from schemas.tasks import SubmitTaskCommand, TaskAcceptedSnapshot
from services.media_generation_inputs import (
    MediaInputFreezer,
    MediaInputNotFound,
    MediaInputOutdated,
    MediaInputRequest,
    UnsupportedMediaUsage,
)
from services.speech_generation_inputs import SpeechInputFreezer
from services.task_submission import TaskSubmitter

__all__ = [
    "GenerateMediaCommand",
    "GenerateMediaHandler",
    "MediaInputNotFound",
    "MediaInputOutdated",
    "UnsupportedMediaUsage",
]


@dataclass(frozen=True, slots=True)
class GenerateMediaCommand:
    episode_id: UUID
    usage_type: UsageType
    usage_id: UUID
    input_version_id: UUID
    idempotency_key: str
    model_profile_id: str | None = None


class GenerateMediaHandler:
    def __init__(
        self,
        database: DatabasePool,
        *,
        release_version: str,
        registry: AiModelRegistry | None = None,
        voices: VoiceCatalog | None = None,
    ) -> None:
        self._database = database
        self._registry = registry or create_mvp_registry()
        self._voices = voices or create_mvp_voice_catalog()
        self._tasks = TaskSubmitter(database, release_version=release_version)
        self._inputs = MediaInputFreezer()
        self._speech_inputs = SpeechInputFreezer()

    async def execute(self, command: GenerateMediaCommand) -> TaskAcceptedSnapshot:
        capability = self._capability(command.usage_type)
        schema_version = f"{capability}-v1"
        profile = self._registry.select(
            capability, command.model_profile_id, schema_version=schema_version
        )
        async with self._database.transaction() as connection:
            if not await self._lock_episode(connection, command.episode_id):
                raise MediaInputNotFound("episode was not found")
            request = MediaInputRequest(
                command.episode_id,
                command.usage_type,
                command.usage_id,
                command.input_version_id,
            )
            freezer = (
                self._speech_inputs
                if command.usage_type == "speech_audio"
                else self._inputs
            )
            frozen = await freezer.freeze(
                connection,
                request,
            )
            if frozen.capability != capability:
                raise RuntimeError("media input capability does not match its usage")
            if capability == "tts":
                self._voices.resolve(
                    profile.provider_id,
                    profile.route_version,
                    cast(VoiceId, frozen.input_refs["voice_id"]),
                )
            input_hash = canonical_content_hash(frozen.input_refs)
            return await self._tasks.submit_in_transaction(
                connection,
                SubmitTaskCommand(
                    episode_id=command.episode_id,
                    task_type="generate_media",
                    capability=capability,
                    scope={
                        "episode_id": str(command.episode_id),
                        "usage_type": command.usage_type,
                        "usage_id": str(command.usage_id),
                    },
                    input_refs={**frozen.input_refs, "input_hash": input_hash},
                    prompt=frozen.prompt,
                    parameters=profile.parameters,
                    model_profile_id=profile.model_profile_id,
                    provider_id=profile.provider_id,
                    model_id=profile.model_id,
                    route_version=profile.route_version,
                    schema_version=profile.schema_version,
                    operation_scope=(
                        f"generateMedia/{command.usage_type}/{command.usage_id}/"
                        f"{command.input_version_id}/{input_hash}"
                    ),
                    idempotency_key=command.idempotency_key,
                    handler_version="1",
                ),
            )

    async def _lock_episode(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        episode_id: UUID,
    ) -> bool:
        value = await connection.fetchval(
            "SELECT true FROM episodes WHERE id=$1 FOR UPDATE", episode_id
        )
        return value is True

    @staticmethod
    def _capability(usage_type: UsageType) -> Capability:
        if usage_type in {"asset_image", "shot_image"}:
            return "image"
        if usage_type == "shot_video":
            return "video"
        if usage_type == "speech_audio":
            return "tts"
        raise UnsupportedMediaUsage("this media usage is not implemented yet")

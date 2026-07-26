from __future__ import annotations

from typing import Protocol, cast
from uuid import UUID

from db.pool import DatabasePool
from integrations.ai.deterministic_media import GeneratedMedia
from integrations.ai.profiles import Capability
from integrations.ai.registry import AiModelBinding, AiModelRegistry
from repositories.task_completion import ExecutionCompletionStore
from repositories.task_executions import ExecutionPlan, MediaExecutionInput, TaskExecutionStore
from repositories.task_outputs import TaskOutputStore
from schemas.media import MediaKind
from schemas.media_registration import (
    MediaRegistrationCommand,
    UsageType,
)
from services.media_registration import MediaRegistrationService
from services.media_validation import InvalidMedia
from workers.dispatch import JobContext
from workers.provider_execution import FaultInjector


class ImageProvider(Protocol):
    async def generate(self, input_hash: str, output_slot: str) -> GeneratedMedia: ...


class VideoProvider(Protocol):
    async def generate(
        self,
        input_hash: str,
        output_slot: str,
        *,
        duration_ticks: int,
    ) -> GeneratedMedia: ...


class InvalidMediaJobInput(ValueError):
    pass


class GenerateMediaJobHandler:
    def __init__(
        self,
        database: DatabasePool,
        *,
        registry: AiModelRegistry,
        registration: MediaRegistrationService,
        fault: FaultInjector,
    ) -> None:
        self._executions = TaskExecutionStore(database)
        self._outputs = TaskOutputStore(database)
        self._completion = ExecutionCompletionStore(database)
        self._registry = registry
        self._registration = registration
        self._fault = fault

    async def handle(self, context: JobContext) -> None:
        plan = await self._executions.prepare(context.payload)
        if plan.skip:
            return
        if plan.cancel_requested:
            await self._completion.mark_cancelled(plan)
            return
        existing = await self._registration.find_registered(plan.attempt_id, "primary")
        if existing is not None:
            if existing.candidate_status != "ready":
                raise RuntimeError("registered primary candidate is not ready")
            await self._complete(plan.task_id, existing.candidate_id, plan)
            return
        job_input = await self._executions.media_input(plan.task_id)
        binding = self._binding(job_input)
        usage_type, usage_id, input_version_id, input_hash, duration = self._usage(
            job_input
        )
        generated = await self._generate(
            binding.adapter,
            cast(Capability, job_input.capability),
            input_hash,
            duration,
        )
        await self._executions.record_provider_success(plan, plan.provider_request_key)
        self._fault.hit("after_media_generation")
        command = MediaRegistrationCommand(
            episode_id=job_input.episode_id,
            task_id=plan.task_id,
            attempt_id=plan.attempt_id,
            output_slot="primary",
            usage_type=usage_type,
            usage_id=usage_id,
            input_version_id=input_version_id,
            input_hash=input_hash,
            media_kind=cast(MediaKind, job_input.capability),
            content_type=generated.content_type,
            data=generated.data,
            target_duration_ticks=duration,
        )
        try:
            registered = await self._registration.register(command)
        except InvalidMedia:
            registered = await self._registration.register_invalid(
                command, reason="OUTPUT_INVALID"
            )
            await self._outputs.record(
                plan.task_id,
                "generation_candidate",
                registered.candidate_id,
                ordinal=0,
            )
            await self._completion.mark_failed(
                plan,
                error_code="OUTPUT_INVALID",
                summary="Provider media failed technical validation",
                retryable=False,
                next_action="Create a new generation task for this slot",
            )
            return
        self._fault.hit("after_media_registration")
        await self._complete(plan.task_id, registered.candidate_id, plan)

    def _binding(self, job_input: MediaExecutionInput) -> AiModelBinding:
        if job_input.task_type != "generate_media" or job_input.capability not in {
            "image",
            "video",
        }:
            raise InvalidMediaJobInput("job is not a supported media generation task")
        capability = cast(Capability, job_input.capability)
        binding = self._registry.bind(capability, job_input.model_profile_id)
        profile = binding.profile
        if (
            profile.provider_id != job_input.provider_id
            or profile.model_id != job_input.model_id
            or profile.route_version != job_input.route_version
            or job_input.schema_version not in profile.schema_versions
        ):
            raise InvalidMediaJobInput("frozen model profile no longer matches the registry")
        return binding

    @staticmethod
    def _usage(
        job_input: MediaExecutionInput,
    ) -> tuple[UsageType, UUID, UUID, str, int | None]:
        refs = job_input.input_refs
        usage_type = refs.get("usage_type")
        allowed = (
            {"asset_image", "shot_image"}
            if job_input.capability == "image"
            else {"shot_video"}
        )
        if usage_type not in allowed:
            raise InvalidMediaJobInput("media usage type is invalid")
        parsed_usage_type = cast(UsageType, usage_type)
        input_hash = refs.get("input_hash")
        usage_id = (
            refs.get("asset_id") if usage_type == "asset_image" else refs.get("shot_id")
        )
        try:
            parsed_usage_id = UUID(str(usage_id))
            input_version_id = UUID(str(refs["input_version_id"]))
        except (KeyError, ValueError) as error:
            raise InvalidMediaJobInput("image usage references are invalid") from error
        if not isinstance(input_hash, str):
            raise InvalidMediaJobInput("media input hash is missing")
        duration = refs.get("duration_ticks") if usage_type == "shot_video" else None
        if duration is not None and (not isinstance(duration, int) or duration <= 0):
            raise InvalidMediaJobInput("video target duration is invalid")
        return parsed_usage_type, parsed_usage_id, input_version_id, input_hash, duration

    @staticmethod
    async def _generate(
        adapter: object,
        capability: Capability,
        input_hash: str,
        duration_ticks: int | None,
    ) -> GeneratedMedia:
        if capability == "image":
            return await cast(ImageProvider, adapter).generate(input_hash, "primary")
        if capability == "video" and duration_ticks is not None:
            return await cast(VideoProvider, adapter).generate(
                input_hash,
                "primary",
                duration_ticks=duration_ticks,
            )
        raise InvalidMediaJobInput("media provider input is incomplete")

    async def _complete(
        self, task_id: UUID, candidate_id: UUID, plan: ExecutionPlan
    ) -> None:
        await self._outputs.record(
            task_id,
            "generation_candidate",
            candidate_id,
            ordinal=0,
        )
        await self._completion.mark_succeeded(plan)

from __future__ import annotations

from typing import cast
from uuid import UUID

from db.pool import DatabasePool
from integrations.ai.profiles import Capability
from integrations.ai.registry import AiModelBinding, AiModelRegistry
from integrations.ai.voices import VoiceCatalog, create_mvp_voice_catalog
from repositories.task_completion import ExecutionCompletionStore
from repositories.task_executions import ExecutionPlan, MediaExecutionInput, TaskExecutionStore
from repositories.task_outputs import TaskOutputStore
from schemas.media_registration import MediaRegistrationCommand
from services.media_registration import MediaRegistrationService
from services.media_validation import InvalidMedia
from workers.dispatch import JobContext
from workers.media_provider import (
    ImageProvider,
    InvalidMediaProviderInput,
    invoke_media_provider,
    parse_media_request,
)
from workers.provider_execution import FaultInjector

__all__ = ["GenerateMediaJobHandler", "ImageProvider"]


class InvalidMediaJobInput(InvalidMediaProviderInput):
    pass


class GenerateMediaJobHandler:
    def __init__(
        self,
        database: DatabasePool,
        *,
        registry: AiModelRegistry,
        registration: MediaRegistrationService,
        fault: FaultInjector,
        voices: VoiceCatalog | None = None,
    ) -> None:
        self._executions = TaskExecutionStore(database)
        self._outputs = TaskOutputStore(database)
        self._completion = ExecutionCompletionStore(database)
        self._registry = registry
        self._registration = registration
        self._fault = fault
        self._voices = voices or create_mvp_voice_catalog()

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
        request = parse_media_request(job_input, plan.prompt)
        provider_voice_id = (
            self._voices.resolve(
                job_input.provider_id,
                job_input.route_version,
                request.logical_voice_id,
            )
            if request.logical_voice_id is not None
            else None
        )
        generated = await invoke_media_provider(
            binding.adapter,
            cast(Capability, job_input.capability),
            request,
            provider_voice_id=provider_voice_id,
        )
        await self._executions.record_provider_success(plan, plan.provider_request_key)
        self._fault.hit("after_media_generation")
        command = MediaRegistrationCommand(
            episode_id=job_input.episode_id,
            task_id=plan.task_id,
            attempt_id=plan.attempt_id,
            output_slot="primary",
            usage_type=request.usage_type,
            usage_id=request.usage_id,
            input_version_id=request.input_version_id,
            input_hash=request.input_hash,
            media_kind=request.media_kind,
            content_type=generated.content_type,
            data=generated.data,
            target_duration_ticks=request.target_duration_ticks,
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
            "tts",
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

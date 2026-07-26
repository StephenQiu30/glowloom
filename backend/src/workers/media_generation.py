from __future__ import annotations

from typing import Protocol, cast
from uuid import UUID

from db.pool import DatabasePool
from integrations.ai.deterministic_media import GeneratedMedia
from integrations.ai.registry import AiModelBinding, AiModelRegistry
from repositories.task_completion import ExecutionCompletionStore
from repositories.task_executions import ExecutionPlan, MediaExecutionInput, TaskExecutionStore
from repositories.task_outputs import TaskOutputStore
from services.media_registration import (
    MediaRegistrationCommand,
    MediaRegistrationService,
    UsageType,
)
from workers.dispatch import JobContext
from workers.provider_execution import FaultInjector


class ImageProvider(Protocol):
    async def generate(self, input_hash: str, output_slot: str) -> GeneratedMedia: ...


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
        provider = cast(ImageProvider, binding.adapter)
        usage_type, usage_id, input_version_id, input_hash = self._usage(job_input)
        generated = await provider.generate(input_hash, "primary")
        await self._executions.record_provider_success(plan, plan.provider_request_key)
        self._fault.hit("after_media_generation")
        registered = await self._registration.register(
            MediaRegistrationCommand(
                episode_id=job_input.episode_id,
                task_id=plan.task_id,
                attempt_id=plan.attempt_id,
                output_slot="primary",
                usage_type=usage_type,
                usage_id=usage_id,
                input_version_id=input_version_id,
                input_hash=input_hash,
                media_kind="image",
                content_type=generated.content_type,
                data=generated.data,
            )
        )
        self._fault.hit("after_media_registration")
        await self._complete(plan.task_id, registered.candidate_id, plan)

    def _binding(self, job_input: MediaExecutionInput) -> AiModelBinding:
        if job_input.task_type != "generate_media" or job_input.capability != "image":
            raise InvalidMediaJobInput("job is not an image generation task")
        binding = self._registry.bind("image", job_input.model_profile_id)
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
    def _usage(job_input: MediaExecutionInput) -> tuple[UsageType, UUID, UUID, str]:
        refs = job_input.input_refs
        usage_type = refs.get("usage_type")
        if usage_type not in {"asset_image", "shot_image"}:
            raise InvalidMediaJobInput("image usage type is invalid")
        input_hash = refs.get("input_hash")
        usage_id = refs.get("asset_id") if usage_type == "asset_image" else refs.get("shot_id")
        try:
            parsed_usage_id = UUID(str(usage_id))
            input_version_id = UUID(str(refs["input_version_id"]))
        except (KeyError, ValueError) as error:
            raise InvalidMediaJobInput("image usage references are invalid") from error
        if not isinstance(input_hash, str):
            raise InvalidMediaJobInput("image input hash is missing")
        return usage_type, parsed_usage_id, input_version_id, input_hash

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

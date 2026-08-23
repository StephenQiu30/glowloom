import asyncio
import inspect
import logging
import time
from typing import Literal, Protocol

import aio_pika
from opentelemetry.trace import SpanKind, Status, StatusCode
from pydantic import ValidationError
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from app.core.config import Settings, get_settings
from app.core.database import session_factory
from app.core.logging import configure_logging, log_event
from app.core.schema import assert_database_schema
from app.core.telemetry import (
    configure_telemetry,
    span_identifiers,
    start_span,
    traceparent_from_headers,
)
from app.integrations.codex_local import CodexLocalScriptStructureExtractor
from app.integrations.deepseek import (
    DeepSeekEpisodePlanner,
    DeepSeekScriptAdapter,
    DeepSeekScriptStructureExtractor,
    DeepSeekStoryboardDrafter,
)
from app.integrations.rabbitmq import declare_task_topology
from app.model_registry import register_implemented_models
from app.modules.messaging import MessageEnvelope
from app.modules.messaging.adaptation_consumer import (
    PreparedScriptAdaptation,
    finalize_script_adaptation_failure,
    finalize_script_adaptation_success,
    prepare_script_adaptation,
)
from app.modules.messaging.consumer import (
    IO_SCRIPT_EXTRACTION_CONSUMER,
    PreparedScriptExtraction,
    consume_envelope,
    finalize_extraction_failure,
    finalize_extraction_success,
    prepare_configured_extraction,
)
from app.modules.messaging.draft_consumer import (
    PreparedStoryboardDraft,
    finalize_storyboard_draft_failure,
    finalize_storyboard_draft_success,
    prepare_storyboard_draft,
)
from app.modules.messaging.generation_consumer import (
    PreparedGenerationDispatch,
    finalize_generation_dispatch_unavailable,
    prepare_generation_dispatch,
)
from app.modules.messaging.metrics import (
    initialize_worker_metrics,
    message_event_type_label,
    observe_message_result,
    track_worker_inflight,
)
from app.modules.messaging.planning_consumer import (
    PreparedEpisodePlanning,
    finalize_episode_planning_failure,
    finalize_episode_planning_success,
    prepare_episode_planning,
)
from app.modules.scripts import (
    ScriptExtractionProviderError,
    ScriptExtractionResult,
    ScriptStructureExtractor,
)
from app.modules.scripts.adaptations import (
    ScriptAdaptationProviderError,
    ScriptAdaptationProviderResult,
    ScriptAdapter,
)
from app.modules.scripts.planning.ports import (
    EpisodePlanner,
    EpisodePlanningProviderError,
)
from app.modules.scripts.planning.schemas import EpisodePlanningProviderResult
from app.modules.storyboards import (
    StoryboardDraftProvider,
    StoryboardDraftProviderError,
)

IO_WORKER_MAX_IN_FLIGHT = 4
MAX_MESSAGE_BYTES = 64 * 1024
WorkerResult = Literal["completed", "duplicate", "rejected", "requeued"]
QUEUE_NAME = "lanverse.io"
LOGGER = logging.getLogger("lanverse.worker")


class IncomingMessage(Protocol):
    body: bytes

    async def ack(self) -> None: ...

    async def nack(self, *, requeue: bool) -> None: ...


@track_worker_inflight(queue=QUEUE_NAME, capacity=IO_WORKER_MAX_IN_FLIGHT)
async def process_incoming_message(
    message: IncomingMessage,
    factory: async_sessionmaker[AsyncSession],
    *,
    extractor: ScriptStructureExtractor | None = None,
    episode_planner: EpisodePlanner | None = None,
    adaptation_provider: ScriptAdapter | None = None,
    storyboard_drafter: StoryboardDraftProvider | None = None,
) -> WorkerResult:
    started = time.perf_counter()
    if len(message.body) > MAX_MESSAGE_BYTES:
        await message.ack()
        _observe_invalid_message(started, error_type="MessageTooLarge")
        return "rejected"
    try:
        envelope = MessageEnvelope.model_validate_json(message.body)
    except ValidationError:
        await message.ack()
        _observe_invalid_message(started, error_type="MessageValidationError")
        return "rejected"

    event_type_label = message_event_type_label(envelope.event_type)
    parent_traceparent = (
        traceparent_from_headers(getattr(message, "headers", None)) or envelope.traceparent
    )
    with start_span(
        "messaging.message.consume",
        kind=SpanKind.CONSUMER,
        parent_traceparent=parent_traceparent,
        attributes={
            "messaging.system": "rabbitmq",
            "messaging.operation": "process",
            "messaging.event.type": event_type_label,
            "messaging.destination.name": QUEUE_NAME,
        },
    ) as span:
        result = await _process_valid_envelope(
            message,
            envelope,
            factory,
            extractor=extractor,
            episode_planner=episode_planner,
            adaptation_provider=adaptation_provider,
            storyboard_drafter=storyboard_drafter,
        )
        span.set_attribute("messaging.operation.result", result)
        if result == "requeued":
            span.set_status(Status(StatusCode.ERROR))
        duration_seconds = time.perf_counter() - started
        trace_id, span_id = span_identifiers(span)
        observe_message_result(
            queue=QUEUE_NAME,
            event_type=envelope.event_type,
            result=result,
            duration_seconds=duration_seconds,
        )
        failed = result in {"rejected", "requeued"}
        common_attributes: dict[str, object] = {
            "request_id": envelope.trace_id,
            "trace_id": trace_id,
            "span_id": span_id,
            "event_id": str(envelope.event_id),
            "event_type": event_type_label,
            "queue": QUEUE_NAME,
            "result": result,
            "duration_ms": round(duration_seconds * 1000, 2),
        }
        if failed:
            log_event(
                LOGGER,
                logging.WARNING,
                "message.consume.failed",
                "message consume failed",
                **common_attributes,
                retryable=result == "requeued",
                error_type=(
                    "MessageProcessingError" if result == "requeued" else "MessageRejected"
                ),
            )
        else:
            log_event(
                LOGGER,
                logging.INFO,
                "message.consume.completed",
                "message consume completed",
                **common_attributes,
            )
        return result


def _observe_invalid_message(started: float, *, error_type: str) -> None:
    duration_seconds = time.perf_counter() - started
    observe_message_result(
        queue=QUEUE_NAME,
        event_type="invalid",
        result="rejected",
        duration_seconds=duration_seconds,
    )
    log_event(
        LOGGER,
        logging.WARNING,
        "message.consume.failed",
        "message consume rejected",
        event_type="invalid",
        queue=QUEUE_NAME,
        result="rejected",
        duration_ms=round(duration_seconds * 1000, 2),
        retryable=False,
        error_type=error_type,
    )


async def _process_valid_envelope(
    message: IncomingMessage,
    envelope: MessageEnvelope,
    factory: async_sessionmaker[AsyncSession],
    *,
    extractor: ScriptStructureExtractor | None,
    episode_planner: EpisodePlanner | None,
    adaptation_provider: ScriptAdapter | None,
    storyboard_drafter: StoryboardDraftProvider | None,
) -> WorkerResult:

    if envelope.event_type == "storyboard_draft.requested":
        return await _process_storyboard_draft_envelope(
            message,
            envelope,
            factory,
            storyboard_drafter=storyboard_drafter,
        )

    if envelope.event_type == "script_adaptation.requested":
        return await _process_script_adaptation_envelope(
            message,
            envelope,
            factory,
            adaptation_provider=adaptation_provider,
        )

    if envelope.event_type == "generation.requested":
        return await _process_generation_envelope(
            message,
            envelope,
            factory,
        )

    if envelope.event_type == "episode_planning.requested":
        return await _process_episode_planning_envelope(
            message,
            envelope,
            factory,
            episode_planner=episode_planner,
        )

    if extractor is None:
        try:
            async with factory() as session:
                async with session.begin():
                    result = await consume_envelope(
                        session,
                        envelope,
                        consumer_name=IO_SCRIPT_EXTRACTION_CONSUMER,
                    )
        except Exception:
            await message.nack(requeue=True)
            return "requeued"

        await message.ack()
        return result

    try:
        async with factory() as session:
            async with session.begin():
                prepared = await prepare_configured_extraction(
                    session,
                    envelope,
                    consumer_name=IO_SCRIPT_EXTRACTION_CONSUMER,
                )
    except Exception:
        await message.nack(requeue=True)
        return "requeued"

    if not isinstance(prepared, PreparedScriptExtraction):
        await message.ack()
        return prepared

    extraction_result: ScriptExtractionResult | None = None
    try:
        extraction_result = await extractor.extract(
            prepared.extraction_input.body,
            trace_id=envelope.trace_id,
            episode_number=prepared.extraction_input.episode_number,
        )
    except ScriptExtractionProviderError as error:
        provider_error = error
    except Exception:
        provider_error = ScriptExtractionProviderError(
            outcome="unknown",
            code="ai_result_unknown",
            summary="DeepSeek response outcome is unknown",
            retryable=False,
            next_action="start_new_extraction",
        )
    else:
        provider_error = None

    try:
        async with factory() as session:
            async with session.begin():
                if provider_error is None:
                    if extraction_result is None:
                        raise RuntimeError("extraction result is unavailable")
                    result = await finalize_extraction_success(
                        session,
                        prepared,
                        extraction_result,
                    )
                else:
                    result = await finalize_extraction_failure(
                        session,
                        prepared,
                        provider_error,
                    )
    except Exception:
        await message.nack(requeue=True)
        return "requeued"

    await message.ack()
    return result


async def _process_generation_envelope(
    message: IncomingMessage,
    envelope: MessageEnvelope,
    factory: async_sessionmaker[AsyncSession],
) -> WorkerResult:
    try:
        async with factory() as session:
            async with session.begin():
                prepared = await prepare_generation_dispatch(session, envelope)
    except Exception:
        await message.nack(requeue=True)
        return "requeued"

    if not isinstance(prepared, PreparedGenerationDispatch):
        await message.ack()
        return prepared

    try:
        async with factory() as session:
            async with session.begin():
                result = await finalize_generation_dispatch_unavailable(
                    session,
                    prepared,
                )
    except Exception:
        await message.nack(requeue=True)
        return "requeued"

    await message.ack()
    return result


async def _process_episode_planning_envelope(
    message: IncomingMessage,
    envelope: MessageEnvelope,
    factory: async_sessionmaker[AsyncSession],
    *,
    episode_planner: EpisodePlanner | None,
) -> WorkerResult:
    try:
        async with factory() as session:
            async with session.begin():
                prepared = await prepare_episode_planning(
                    session,
                    envelope,
                    configured=episode_planner is not None,
                )
    except Exception:
        await message.nack(requeue=True)
        return "requeued"

    if not isinstance(prepared, PreparedEpisodePlanning):
        await message.ack()
        return prepared
    if episode_planner is None:
        await message.nack(requeue=True)
        return "requeued"

    planning_result: EpisodePlanningProviderResult | None = None
    try:
        planning_result = await episode_planner.plan(
            prepared.planning_input.normalized_text,
            target_duration_ms=prepared.planning_input.target_duration_ms,
            maximum_episode_count=prepared.planning_input.maximum_episode_count,
        )
    except EpisodePlanningProviderError as error:
        provider_error = error
    except Exception:
        provider_error = EpisodePlanningProviderError(
            outcome="unknown",
            code="ai_result_unknown",
            summary="DeepSeek response outcome is unknown",
            retryable=False,
            next_action="start_new_episode_plan",
        )
    else:
        provider_error = None

    try:
        async with factory() as session:
            async with session.begin():
                if provider_error is None:
                    if planning_result is None:
                        raise RuntimeError("episode planning result is unavailable")
                    result = await finalize_episode_planning_success(
                        session,
                        prepared,
                        planning_result,
                    )
                else:
                    result = await finalize_episode_planning_failure(
                        session,
                        prepared,
                        provider_error,
                    )
    except Exception:
        await message.nack(requeue=True)
        return "requeued"

    await message.ack()
    return result


async def _process_script_adaptation_envelope(
    message: IncomingMessage,
    envelope: MessageEnvelope,
    factory: async_sessionmaker[AsyncSession],
    *,
    adaptation_provider: ScriptAdapter | None,
) -> WorkerResult:
    try:
        async with factory() as session:
            async with session.begin():
                prepared = await prepare_script_adaptation(
                    session,
                    envelope,
                    configured=adaptation_provider is not None,
                )
    except Exception:
        await message.nack(requeue=True)
        return "requeued"

    if not isinstance(prepared, PreparedScriptAdaptation):
        await message.ack()
        return prepared
    if adaptation_provider is None:
        await message.nack(requeue=True)
        return "requeued"

    adaptation_result: ScriptAdaptationProviderResult | dict[str, object] | None = None
    try:
        adaptation_result = await adaptation_provider.adapt(
            prepared.adaptation_input.script_body,
            target_duration_ms=prepared.adaptation_input.target_duration_ms,
            core_plot_points=prepared.adaptation_input.core_plot_points,
            pacing=prepared.adaptation_input.pacing,
            colloquial_dialogue=prepared.adaptation_input.colloquial_dialogue,
        )
    except ScriptAdaptationProviderError as error:
        provider_error = error
    except Exception:
        provider_error = ScriptAdaptationProviderError(
            outcome="unknown",
            code="ai_result_unknown",
            summary="AI adaptation response outcome is unknown",
            retryable=False,
            next_action="start_new_adaptation",
        )
    else:
        provider_error = None

    try:
        async with factory() as session:
            async with session.begin():
                if provider_error is None:
                    if adaptation_result is None:
                        raise RuntimeError("adaptation result is unavailable")
                    result = await finalize_script_adaptation_success(
                        session,
                        prepared,
                        adaptation_result,
                    )
                else:
                    result = await finalize_script_adaptation_failure(
                        session,
                        prepared,
                        provider_error,
                    )
    except Exception:
        await message.nack(requeue=True)
        return "requeued"

    await message.ack()
    return result


async def _process_storyboard_draft_envelope(
    message: IncomingMessage,
    envelope: MessageEnvelope,
    factory: async_sessionmaker[AsyncSession],
    *,
    storyboard_drafter: StoryboardDraftProvider | None,
) -> WorkerResult:
    try:
        async with factory() as session:
            async with session.begin():
                prepared = await prepare_storyboard_draft(
                    session,
                    envelope,
                    configured=storyboard_drafter is not None,
                )
    except Exception:
        await message.nack(requeue=True)
        return "requeued"

    if not isinstance(prepared, PreparedStoryboardDraft):
        await message.ack()
        return prepared
    if storyboard_drafter is None:
        await message.nack(requeue=True)
        return "requeued"

    draft_result: dict[str, object] | None = None
    try:
        draft_result = await storyboard_drafter.draft(prepared.draft_input)
    except StoryboardDraftProviderError as error:
        provider_error = error
    except Exception:
        provider_error = StoryboardDraftProviderError(
            outcome="unknown",
            code="ai_result_unknown",
            summary="AI storyboard draft response outcome is unknown",
            retryable=False,
            next_action="create_new_storyboard_draft_batch",
        )
    else:
        provider_error = None

    try:
        async with factory() as session:
            async with session.begin():
                if provider_error is None:
                    if draft_result is None:
                        raise RuntimeError("storyboard draft result is unavailable")
                    result = await finalize_storyboard_draft_success(
                        session,
                        prepared,
                        draft_result,
                    )
                else:
                    result = await finalize_storyboard_draft_failure(
                        session,
                        prepared,
                        provider_error,
                    )
    except Exception:
        await message.nack(requeue=True)
        return "requeued"

    await message.ack()
    return result


async def run_io_worker(settings: Settings) -> None:
    configure_logging(
        settings.log_level,
        service="lanverse-io-worker",
        environment=settings.environment,
    )
    configure_telemetry(
        service_name="lanverse-io-worker",
        environment=settings.environment,
    )
    register_implemented_models()
    await assert_database_schema()
    initialize_worker_metrics(queue=QUEUE_NAME, capacity=IO_WORKER_MAX_IN_FLIGHT)
    if settings.script_extraction_provider == "codex_local":
        extractor = CodexLocalScriptStructureExtractor(
            codex_cli_path=settings.codex_cli_path,
            model=settings.codex_model,
            max_concurrency=settings.codex_max_concurrency,
        )
    elif (
        settings.script_extraction_provider == "deepseek" and settings.deepseek_api_key is not None
    ):
        extractor = DeepSeekScriptStructureExtractor(settings.deepseek_api_key)
    else:
        extractor = None
    episode_planner = (
        DeepSeekEpisodePlanner(settings.deepseek_api_key)
        if settings.deepseek_api_key is not None
        else None
    )
    adaptation_provider = (
        DeepSeekScriptAdapter(settings.deepseek_api_key)
        if settings.deepseek_api_key is not None
        else None
    )
    storyboard_drafter = (
        DeepSeekStoryboardDrafter(settings.deepseek_api_key)
        if settings.deepseek_api_key is not None
        else None
    )
    connection = await aio_pika.connect_robust(settings.rabbitmq_url, timeout=3)
    try:
        channel = await connection.channel()
        await channel.set_qos(prefetch_count=IO_WORKER_MAX_IN_FLIGHT)
        _, io_queue, _ = await declare_task_topology(channel)

        async def on_message(message: aio_pika.abc.AbstractIncomingMessage) -> None:
            await process_incoming_message(
                message,
                session_factory,
                extractor=extractor,
                episode_planner=episode_planner,
                adaptation_provider=adaptation_provider,
                storyboard_drafter=storyboard_drafter,
            )

        await io_queue.consume(on_message, no_ack=False)
        await asyncio.Future()
    finally:
        await connection.close()
        for provider in (
            extractor,
            episode_planner,
            adaptation_provider,
            storyboard_drafter,
        ):
            close = getattr(provider, "aclose", None)
            if close is not None:
                result = close()
                if inspect.isawaitable(result):
                    await result


def main() -> None:
    asyncio.run(run_io_worker(get_settings()))


if __name__ == "__main__":
    main()

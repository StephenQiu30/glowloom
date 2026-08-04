import asyncio
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
from app.core.telemetry import (
    configure_telemetry,
    span_identifiers,
    start_span,
    traceparent_from_headers,
)
from app.integrations.deepseek import DeepSeekScriptStructureExtractor
from app.integrations.rabbitmq import declare_task_topology
from app.model_registry import register_implemented_models
from app.modules.messaging import MessageEnvelope
from app.modules.messaging.consumer import (
    IO_SCRIPT_EXTRACTION_CONSUMER,
    PreparedScriptExtraction,
    consume_envelope,
    finalize_extraction_failure,
    finalize_extraction_success,
    prepare_configured_extraction,
)
from app.modules.messaging.metrics import (
    message_event_type_label,
    observe_message_result,
)
from app.modules.scripts import (
    ScriptExtractionProviderError,
    ScriptExtractionResult,
    ScriptStructureExtractor,
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


async def process_incoming_message(
    message: IncomingMessage,
    factory: async_sessionmaker[AsyncSession],
    *,
    extractor: ScriptStructureExtractor | None = None,
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
) -> WorkerResult:

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
        extraction_result = await extractor.extract(prepared.extraction_input.body)
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
    extractor = (
        DeepSeekScriptStructureExtractor(settings.deepseek_api_key)
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
            )

        await io_queue.consume(on_message, no_ack=False)
        await asyncio.Future()
    finally:
        await connection.close()


def main() -> None:
    asyncio.run(run_io_worker(get_settings()))


if __name__ == "__main__":
    main()

import asyncio
import os
from datetime import timedelta
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from threading import Thread
from uuid import uuid4

import pytest
from temporalio import activity, workflow
from temporalio.client import Client
from temporalio.worker import Worker

# Temporal reimports this module in the workflow sandbox.
# Import the Worker entrypoint inside tests to keep HTTP clients out of the sandbox.


def test_trace_provider_propagates_without_collector() -> None:
    from app.main_worker import create_trace_provider

    provider = create_trace_provider(None)
    try:
        with provider.get_tracer("test").start_as_current_span("request") as span:
            assert span.get_span_context().is_valid
    finally:
        provider.shutdown()


def test_trace_provider_exports_otlp_http_and_flushes_on_shutdown() -> None:
    from app.main_worker import create_trace_provider

    requests: list[tuple[str, str, bytes]] = []

    class Handler(BaseHTTPRequestHandler):
        def do_POST(self) -> None:
            body = self.rfile.read(int(self.headers["Content-Length"]))
            requests.append((self.path, self.headers["Content-Type"], body))
            self.send_response(200)
            self.end_headers()

        def log_message(self, _format: str, *_args: object) -> None:
            pass

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = Thread(target=server.serve_forever)
    thread.start()
    try:
        provider = create_trace_provider(f"http://127.0.0.1:{server.server_port}")
        with provider.get_tracer("test").start_as_current_span("agent-test") as span:
            trace_id = span.get_span_context().trace_id
        provider.shutdown()
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=5)

    from opentelemetry.proto.collector.trace.v1.trace_service_pb2 import (
        ExportTraceServiceRequest,
    )

    assert len(requests) == 1
    path, content_type, body = requests[0]
    assert path == "/v1/traces"
    assert content_type == "application/x-protobuf"
    exported = ExportTraceServiceRequest.FromString(body)
    resource_spans = exported.resource_spans
    assert len(resource_spans) == 1
    assert any(
        attribute.key == "service.name" and attribute.value.string_value == "lanverse-agent-worker"
        for attribute in resource_spans[0].resource.attributes
    )
    spans = [span for scope in resource_spans[0].scope_spans for span in scope.spans]
    assert len(spans) == 1
    assert spans[0].trace_id == trace_id.to_bytes(16, "big")


@workflow.defn
class TraceProbeWorkflow:
    @workflow.run
    async def run(self, activity_queue: str) -> str:
        result = await workflow.execute_activity(
            "trace.probe",
            task_queue=activity_queue,
            start_to_close_timeout=timedelta(seconds=10),
            result_type=str,
        )
        assert isinstance(result, str)
        return result


@activity.defn(name="trace.probe")
async def trace_probe() -> str:
    from opentelemetry import trace

    return f"{trace.get_current_span().get_span_context().trace_id:032x}"


def test_local_temporal_activity_inherits_parent_trace() -> None:
    from app.main_worker import create_trace_provider

    temporal_addr = os.getenv("LV_TEST_TEMPORAL_ADDR")
    namespace = os.getenv("LV_TEST_TEMPORAL_NAMESPACE")
    if not temporal_addr or not namespace:
        pytest.skip("set LV_TEST_TEMPORAL_ADDR and LV_TEST_TEMPORAL_NAMESPACE")

    async def run() -> None:
        from temporalio.contrib.opentelemetry import TracingInterceptor

        provider = create_trace_provider(None)
        try:
            client = await Client.connect(
                temporal_addr,
                namespace=namespace,
                interceptors=[TracingInterceptor(tracer=provider.get_tracer("agent-worker-test"))],
            )
            token = uuid4().hex
            flow_queue = f"lanverse-trace-flow-{token}"
            activity_queue = f"lanverse-trace-agent-{token}"
            async with (
                Worker(client, task_queue=flow_queue, workflows=[TraceProbeWorkflow]),
                Worker(client, task_queue=activity_queue, activities=[trace_probe]),
            ):
                with provider.get_tracer("test").start_as_current_span("request") as parent:
                    expected = f"{parent.get_span_context().trace_id:032x}"
                    actual = await asyncio.wait_for(
                        client.execute_workflow(
                            TraceProbeWorkflow.run,
                            activity_queue,
                            id=f"lanverse-trace-{token}",
                            task_queue=flow_queue,
                        ),
                        timeout=30,
                    )
            assert actual == expected
        finally:
            provider.shutdown()

    asyncio.run(run())

from uuid import UUID

import httpx
import pytest
from uuid6 import uuid7

from app.main import create_app


@pytest.mark.asyncio
async def test_health_endpoint_returns_request_id() -> None:
    request_id = str(uuid7())
    incoming_traceparent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
    transport = httpx.ASGITransport(app=create_app())
    async with httpx.AsyncClient(transport=transport, base_url="http://test") as client:
        response = await client.get(
            "/healthz",
            headers={
                "x-request-id": request_id,
                "traceparent": incoming_traceparent,
            },
        )
    assert response.status_code == 200
    assert response.json() == {"status": "ok"}
    assert response.headers["x-request-id"] == request_id
    response_traceparent = response.headers["traceparent"]
    assert response_traceparent.split("-")[1] == incoming_traceparent.split("-")[1]
    assert response_traceparent.split("-")[2] != incoming_traceparent.split("-")[2]


@pytest.mark.asyncio
async def test_health_endpoint_replaces_non_uuid7_request_id() -> None:
    transport = httpx.ASGITransport(app=create_app())
    async with httpx.AsyncClient(transport=transport, base_url="http://test") as client:
        response = await client.get(
            "/healthz",
            headers={
                "x-request-id": "not-a-canonical-uuid7",
                "traceparent": "not-a-valid-traceparent",
            },
        )

    response_request_id = response.headers["x-request-id"]
    assert response_request_id != "not-a-canonical-uuid7"
    parsed_request_id = UUID(response_request_id)
    assert str(parsed_request_id) == response_request_id
    assert parsed_request_id.version == 7
    response_traceparent = response.headers["traceparent"]
    version, trace_id, span_id, flags = response_traceparent.split("-")
    assert version == "00"
    assert len(trace_id) == 32
    assert int(trace_id, 16) != 0
    assert len(span_id) == 16
    assert int(span_id, 16) != 0
    assert len(flags) == 2
    assert 0 <= int(flags, 16) <= 255

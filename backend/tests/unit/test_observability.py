import logging
from typing import Protocol, cast

import httpx
import pytest
from uuid6 import uuid7

from app.main import create_app


class _AccessLogRecord(Protocol):
    context: dict[str, object]


@pytest.mark.asyncio
async def test_access_log_and_metrics_use_route_template_and_status_class(
    caplog: pytest.LogCaptureFixture,
) -> None:
    resource_id = uuid7()
    request_id = str(uuid7())
    caplog.set_level(logging.INFO, logger="lanverse.http")
    transport = httpx.ASGITransport(app=create_app())

    async with httpx.AsyncClient(transport=transport, base_url="http://test") as client:
        response = await client.get(
            f"/api/v1/tasks/{resource_id}",
            headers={"x-request-id": request_id},
        )
        metrics = await client.get("/metrics")

    assert response.status_code == 401
    access_record = next(
        record
        for record in caplog.records
        if getattr(record, "event_name", None) == "http.request.completed"
        and getattr(record, "context", {}).get("request_id") == request_id
    )
    context = cast(_AccessLogRecord, access_record).context
    assert context["route"] == "/api/v1/tasks/{task_id}"
    assert context["status_class"] == "4xx"
    assert str(resource_id) not in str(context)

    rendered_metrics = metrics.text
    assert (
        'lanverse_http_requests_total{method="GET",route="/api/v1/tasks/{task_id}",status_class="4xx"}'
        in rendered_metrics
    )
    assert str(resource_id) not in rendered_metrics

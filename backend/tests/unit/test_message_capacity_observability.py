import asyncio
from datetime import UTC, datetime, timedelta

import pytest
from prometheus_client import REGISTRY

from app.modules.messaging.metrics import (
    initialize_worker_metrics,
    observe_outbox_backlog,
    track_worker_inflight,
)
from app.modules.messaging.service import OutboxBacklog


class _BrokenGauge:
    def labels(self, **_: str) -> "_BrokenGauge":
        return self

    def set(self, _value: float) -> None:
        raise RuntimeError("metrics backend failed")

    def inc(self) -> None:
        raise RuntimeError("metrics backend failed")

    def dec(self) -> None:
        raise RuntimeError("metrics backend failed")


def _sample(name: str, labels: dict[str, str]) -> float:
    value = REGISTRY.get_sample_value(name, labels)
    assert value is not None
    return value


def test_outbox_backlog_metrics_are_bounded_and_clear_stale_values() -> None:
    observed_at = datetime(2026, 8, 4, 12, 0, tzinfo=UTC)
    hostile_topic = "tenant.private.route.9f6297b8"
    observe_outbox_backlog(
        [
            OutboxBacklog(
                topic="lanverse.io.v1",
                state="pending",
                count=2,
                oldest_created_at=observed_at - timedelta(seconds=10),
            ),
            OutboxBacklog(
                topic="lanverse.media.v1",
                state="claimed",
                count=1,
                oldest_created_at=observed_at - timedelta(seconds=3),
            ),
            OutboxBacklog(
                topic=hostile_topic,
                state="manual_attention",
                count=4,
                oldest_created_at=observed_at + timedelta(seconds=5),
            ),
        ],
        observed_at=observed_at,
    )

    assert (
        _sample(
            "lanverse_outbox_events",
            {"topic": "lanverse.io.v1", "state": "pending"},
        )
        == 2
    )
    assert (
        _sample(
            "lanverse_outbox_oldest_age_seconds",
            {"topic": "lanverse.io.v1", "state": "pending"},
        )
        == 10
    )
    assert (
        _sample(
            "lanverse_outbox_events",
            {"topic": "lanverse.media.v1", "state": "claimed"},
        )
        == 1
    )
    assert (
        _sample(
            "lanverse_outbox_events",
            {"topic": "unregistered", "state": "manual_attention"},
        )
        == 4
    )
    assert (
        _sample(
            "lanverse_outbox_oldest_age_seconds",
            {"topic": "unregistered", "state": "manual_attention"},
        )
        == 0
    )

    observe_outbox_backlog([], observed_at=observed_at)
    for topic in ("lanverse.io.v1", "lanverse.media.v1", "unregistered"):
        for state in ("pending", "claimed", "manual_attention"):
            assert (
                _sample(
                    "lanverse_outbox_events",
                    {"topic": topic, "state": state},
                )
                == 0
            )
            assert (
                _sample(
                    "lanverse_outbox_oldest_age_seconds",
                    {"topic": topic, "state": state},
                )
                == 0
            )


@pytest.mark.asyncio
async def test_worker_inflight_tracks_concurrency_and_always_returns_to_zero() -> None:
    both_started = asyncio.Event()
    release = asyncio.Event()
    started = 0

    initialize_worker_metrics(topic="lanverse.media.v1", capacity=2)

    @track_worker_inflight(topic="lanverse.media.v1", capacity=2)
    async def blocked_handler() -> str:
        nonlocal started
        started += 1
        if started == 2:
            both_started.set()
        await release.wait()
        return "completed"

    first = asyncio.create_task(blocked_handler())
    second = asyncio.create_task(blocked_handler())
    await asyncio.wait_for(both_started.wait(), timeout=1)
    assert _sample("lanverse_worker_inflight", {"topic": "lanverse.media.v1"}) == 2
    assert _sample("lanverse_worker_capacity", {"topic": "lanverse.media.v1"}) == 2

    release.set()
    assert await asyncio.gather(first, second) == ["completed", "completed"]
    assert _sample("lanverse_worker_inflight", {"topic": "lanverse.media.v1"}) == 0


@pytest.mark.asyncio
async def test_capacity_metric_failures_do_not_escape_business_handlers(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    from app.modules.messaging import metrics

    monkeypatch.setattr(metrics, "OUTBOX_EVENTS", _BrokenGauge())
    monkeypatch.setattr(metrics, "OUTBOX_OLDEST_AGE", _BrokenGauge())
    monkeypatch.setattr(metrics, "WORKER_INFLIGHT", _BrokenGauge())
    monkeypatch.setattr(metrics, "WORKER_CAPACITY", _BrokenGauge())

    observe_outbox_backlog([], observed_at=datetime.now(UTC))
    initialize_worker_metrics(topic="lanverse.io.v1", capacity=4)

    @track_worker_inflight(topic="lanverse.io.v1", capacity=4)
    async def handler() -> str:
        return "business-result"

    assert await handler() == "business-result"

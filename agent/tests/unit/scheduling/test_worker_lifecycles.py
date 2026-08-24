from collections.abc import Awaitable, Callable

import pytest

from app.runtime.workers.scheduler import (
    parse_outbox_topics,
    run_outbox_publisher,
    run_schedule_dispatcher,
    scheduler_service,
)


def test_scheduler_process_modes_have_independent_lifecycles() -> None:
    services: dict[str, Callable[..., Awaitable[None]]] = {
        "schedule": run_schedule_dispatcher,
        "outbox": run_outbox_publisher,
    }

    assert scheduler_service("schedule") is services["schedule"]
    assert scheduler_service("outbox") is services["outbox"]
    assert services["schedule"] is not services["outbox"]


def test_outbox_topic_allowlist_rejects_unknown_or_empty_lanes() -> None:
    assert parse_outbox_topics("lanverse.io.v1, lanverse.media.v1") == frozenset(
        {"lanverse.io.v1", "lanverse.media.v1"}
    )

    with pytest.raises(ValueError, match="must not be empty"):
        parse_outbox_topics(" , ")
    with pytest.raises(ValueError, match="not registered"):
        parse_outbox_topics("lanverse.audit.v2")

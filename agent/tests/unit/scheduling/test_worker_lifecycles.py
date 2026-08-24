from collections.abc import Awaitable, Callable

from app.runtime.workers.scheduler import (
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

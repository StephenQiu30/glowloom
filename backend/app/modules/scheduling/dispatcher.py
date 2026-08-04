import logging
from datetime import datetime, timedelta

from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from app.modules.scheduling import repository, service
from app.modules.scheduling.metrics import (
    SCHEDULE_DISPATCH_RESULTS,
    SCHEDULE_LAG_SECONDS,
    SCHEDULE_MANUAL_ATTENTION,
    SCHEDULE_MISFIRES,
)

logger = logging.getLogger("lanverse.scheduler")


async def dispatch_due_schedules(
    factory: async_sessionmaker[AsyncSession],
    *,
    dispatcher_id: str,
    now: datetime | None,
    batch_size: int,
    lease_duration: timedelta,
) -> int:
    async with factory() as session:
        async with session.begin():
            effective_now = now or await repository.current_database_time(session)
            schedule_ids = await repository.claim_due_schedules(
                session,
                dispatcher_id=dispatcher_id,
                now=effective_now,
                batch_size=batch_size,
                lease_duration=lease_duration,
            )

    dispatched = 0
    for schedule_id in schedule_ids:
        try:
            async with factory() as session:
                async with session.begin():
                    outcome = await service.dispatch_claimed_schedule(
                        session,
                        schedule_id,
                        dispatcher_id=dispatcher_id,
                        now=effective_now,
                    )
        except Exception as error:
            async with factory() as session:
                async with session.begin():
                    failure = await service.record_dispatch_failure(
                        session,
                        schedule_id,
                        dispatcher_id=dispatcher_id,
                        now=effective_now,
                        error=error,
                    )
            if failure.handler is not None:
                SCHEDULE_DISPATCH_RESULTS.labels(
                    handler=failure.handler,
                    result="failed",
                ).inc()
                if failure.reason is not None:
                    SCHEDULE_MANUAL_ATTENTION.labels(
                        handler=failure.handler,
                        reason=failure.reason,
                    ).inc()
            logger.error(
                "schedule dispatch stopped for operator action"
                if failure.manual_attention
                else "schedule dispatch will retry",
                extra={
                    "schedule_id": str(schedule_id),
                    "dispatcher_id": dispatcher_id,
                    "error_type": type(error).__name__,
                },
            )
        else:
            if outcome.handler is not None and outcome.result is not None:
                SCHEDULE_LAG_SECONDS.labels(handler=outcome.handler).observe(
                    outcome.lag_seconds
                )
                if outcome.misfire_count and outcome.misfire_policy is not None:
                    SCHEDULE_MISFIRES.labels(
                        handler=outcome.handler,
                        policy=outcome.misfire_policy,
                    ).inc(outcome.misfire_count)
                SCHEDULE_DISPATCH_RESULTS.labels(
                    handler=outcome.handler,
                    result=outcome.result,
                ).inc(outcome.metric_count)
            dispatched += outcome.dispatched_count
    return dispatched

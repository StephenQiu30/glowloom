from datetime import UTC, datetime, timedelta

import pytest

from app.modules.scheduling.rules import (
    InvalidScheduleRule,
    next_cron_occurrence,
    plan_due_occurrences,
)


def test_cron_skips_dst_gap_and_only_uses_first_fold() -> None:
    before_spring_gap = datetime(2026, 3, 7, 7, 30, tzinfo=UTC)
    assert next_cron_occurrence(
        "30 2 * * *", "America/New_York", after=before_spring_gap
    ) == datetime(2026, 3, 9, 6, 30, tzinfo=UTC)

    before_fall_fold = datetime(2026, 10, 31, 5, 30, tzinfo=UTC)
    first_fold = next_cron_occurrence(
        "30 1 * * *", "America/New_York", after=before_fall_fold
    )
    assert first_fold == datetime(2026, 11, 1, 5, 30, tzinfo=UTC)
    assert next_cron_occurrence(
        "30 1 * * *", "America/New_York", after=first_fold
    ) == datetime(2026, 11, 2, 6, 30, tzinfo=UTC)


@pytest.mark.parametrize(
    ("expression", "timezone"),
    [
        ("* * * *", "UTC"),
        ("0 0 0 * *", "UTC"),
        ("@daily", "UTC"),
        ("0 0 * JAN *", "UTC"),
        ("0 0 * * *", "Not/AZone"),
    ],
)
def test_cron_rejects_unregistered_syntax_and_timezones(
    expression: str,
    timezone: str,
) -> None:
    with pytest.raises(InvalidScheduleRule):
        next_cron_occurrence(
            expression,
            timezone,
            after=datetime(2026, 1, 1, tzinfo=UTC),
        )


def test_cron_supports_numeric_lists_ranges_and_steps() -> None:
    assert next_cron_occurrence(
        "*/15 9-10 * * 1,3,5",
        "Asia/Shanghai",
        after=datetime(2026, 8, 3, 1, 1, tzinfo=UTC),
    ) == datetime(2026, 8, 3, 1, 15, tzinfo=UTC)


def test_interval_misfire_policies_are_bounded_and_advance_past_now() -> None:
    start = datetime(2026, 8, 4, 0, 0, tzinfo=UTC)
    now = start + timedelta(minutes=5, seconds=45)
    rule = {"seconds": 60, "misfire_grace_seconds": 30}

    skipped = plan_due_occurrences(
        kind="interval",
        rule=rule,
        timezone="UTC",
        next_fire_at=start,
        misfire_policy="skip",
        max_catch_up=0,
        now=now,
    )
    assert skipped.fire_times == ()
    assert skipped.misfire_count == 6
    assert skipped.next_fire_at == start + timedelta(minutes=6)

    run_once = plan_due_occurrences(
        kind="interval",
        rule=rule,
        timezone="UTC",
        next_fire_at=start,
        misfire_policy="run_once",
        max_catch_up=0,
        now=now,
    )
    assert run_once.fire_times == (start + timedelta(minutes=5),)
    assert run_once.misfire_count == 6
    assert run_once.next_fire_at == start + timedelta(minutes=6)

    caught_up = plan_due_occurrences(
        kind="interval",
        rule=rule,
        timezone="UTC",
        next_fire_at=start,
        misfire_policy="catch_up",
        max_catch_up=3,
        now=now,
    )
    assert caught_up.fire_times == (
        start + timedelta(minutes=3),
        start + timedelta(minutes=4),
        start + timedelta(minutes=5),
    )
    assert caught_up.misfire_count == 6
    assert caught_up.next_fire_at == start + timedelta(minutes=6)


def test_skip_keeps_an_on_time_occurrence_but_drops_older_history() -> None:
    start = datetime(2026, 8, 4, 0, 0, tzinfo=UTC)
    plan = plan_due_occurrences(
        kind="interval",
        rule={"seconds": 60, "misfire_grace_seconds": 30},
        timezone="UTC",
        next_fire_at=start,
        misfire_policy="skip",
        max_catch_up=0,
        now=start + timedelta(minutes=5, seconds=10),
    )
    assert plan.fire_times == (start + timedelta(minutes=5),)
    assert plan.misfire_count == 5
    assert plan.next_fire_at == start + timedelta(minutes=6)


def test_missed_one_off_skip_completes_without_a_fire() -> None:
    scheduled_for = datetime(2026, 8, 4, 0, 0, tzinfo=UTC)
    skipped = plan_due_occurrences(
        kind="one_off",
        rule={
            "at": scheduled_for.isoformat(),
            "misfire_grace_seconds": 30,
        },
        timezone="UTC",
        next_fire_at=scheduled_for,
        misfire_policy="skip",
        max_catch_up=0,
        now=scheduled_for + timedelta(minutes=1),
    )
    assert skipped.fire_times == ()
    assert skipped.next_fire_at is None
    assert skipped.completed is True
    assert skipped.misfire_count == 1

    run_once = plan_due_occurrences(
        kind="one_off",
        rule={
            "at": scheduled_for.isoformat(),
            "misfire_grace_seconds": 30,
        },
        timezone="UTC",
        next_fire_at=scheduled_for,
        misfire_policy="run_once",
        max_catch_up=0,
        now=scheduled_for + timedelta(minutes=1),
    )
    assert run_once.fire_times == (scheduled_for,)
    assert run_once.next_fire_at is None
    assert run_once.completed is True

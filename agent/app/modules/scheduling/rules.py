from __future__ import annotations

import math
from dataclasses import dataclass
from datetime import UTC, date, datetime, time, timedelta
from typing import Any, Literal
from zoneinfo import ZoneInfo, ZoneInfoNotFoundError


class InvalidScheduleRule(ValueError):
    """Raised when a persisted schedule rule cannot be executed safely."""


@dataclass(frozen=True)
class DispatchPlan:
    fire_times: tuple[datetime, ...]
    next_fire_at: datetime | None
    completed: bool
    misfire_count: int


@dataclass(frozen=True)
class _CronSpec:
    minutes: frozenset[int]
    hours: frozenset[int]
    month_days: frozenset[int]
    months: frozenset[int]
    week_days: frozenset[int]
    month_days_wildcard: bool
    week_days_wildcard: bool


def _require_aware(value: datetime, field: str) -> datetime:
    if value.tzinfo is None or value.utcoffset() is None:
        raise InvalidScheduleRule(f"{field} must include a timezone")
    return value.astimezone(UTC)


def _parse_number(value: str, minimum: int, maximum: int, field: str) -> int:
    if not value.isascii() or not value.isdigit():
        raise InvalidScheduleRule(f"invalid {field} field")
    parsed = int(value)
    if parsed < minimum or parsed > maximum:
        raise InvalidScheduleRule(f"invalid {field} field")
    return parsed


def _parse_cron_field(
    raw: str,
    *,
    minimum: int,
    maximum: int,
    field: str,
    sunday_alias: bool = False,
) -> frozenset[int]:
    if not raw:
        raise InvalidScheduleRule(f"invalid {field} field")
    values: set[int] = set()
    for item in raw.split(","):
        if not item or item.count("/") > 1:
            raise InvalidScheduleRule(f"invalid {field} field")
        base, separator, step_raw = item.partition("/")
        step = _parse_number(step_raw, 1, maximum - minimum + 1, field) if separator else 1
        if base == "*":
            start, end = minimum, maximum
        elif "-" in base:
            if base.count("-") != 1:
                raise InvalidScheduleRule(f"invalid {field} field")
            start_raw, end_raw = base.split("-", maxsplit=1)
            start = _parse_number(start_raw, minimum, maximum, field)
            end = _parse_number(end_raw, minimum, maximum, field)
            if end < start:
                raise InvalidScheduleRule(f"invalid {field} field")
        else:
            if separator:
                raise InvalidScheduleRule(f"invalid {field} field")
            start = end = _parse_number(base, minimum, maximum, field)
        for value in range(start, end + 1, step):
            values.add(0 if sunday_alias and value == 7 else value)
    if not values:
        raise InvalidScheduleRule(f"invalid {field} field")
    return frozenset(values)


def _parse_cron(expression: str) -> _CronSpec:
    parts = expression.split()
    if len(parts) != 5:
        raise InvalidScheduleRule("cron expression must contain five numeric fields")
    minute, hour, month_day, month, week_day = parts
    return _CronSpec(
        minutes=_parse_cron_field(minute, minimum=0, maximum=59, field="minute"),
        hours=_parse_cron_field(hour, minimum=0, maximum=23, field="hour"),
        month_days=_parse_cron_field(month_day, minimum=1, maximum=31, field="day-of-month"),
        months=_parse_cron_field(month, minimum=1, maximum=12, field="month"),
        week_days=_parse_cron_field(
            week_day,
            minimum=0,
            maximum=7,
            field="day-of-week",
            sunday_alias=True,
        ),
        month_days_wildcard=month_day == "*",
        week_days_wildcard=week_day == "*",
    )


def _zone(timezone: str) -> ZoneInfo:
    try:
        return ZoneInfo(timezone)
    except (ZoneInfoNotFoundError, ValueError) as error:
        raise InvalidScheduleRule("unknown IANA timezone") from error


def _date_matches(spec: _CronSpec, candidate: date) -> bool:
    if candidate.month not in spec.months:
        return False
    month_day_match = candidate.day in spec.month_days
    cron_week_day = (candidate.weekday() + 1) % 7
    week_day_match = cron_week_day in spec.week_days
    if spec.month_days_wildcard and spec.week_days_wildcard:
        return True
    if spec.month_days_wildcard:
        return week_day_match
    if spec.week_days_wildcard:
        return month_day_match
    return month_day_match or week_day_match


def _wall_time_to_first_fold_utc(
    candidate_date: date,
    hour: int,
    minute: int,
    zone: ZoneInfo,
) -> datetime | None:
    naive = datetime.combine(candidate_date, time(hour=hour, minute=minute))
    local = naive.replace(tzinfo=zone, fold=0)
    utc_value = local.astimezone(UTC)
    round_trip = utc_value.astimezone(zone)
    if round_trip.replace(tzinfo=None) != naive or round_trip.fold != 0:
        return None
    return utc_value


def next_cron_occurrence(
    expression: str,
    timezone: str,
    *,
    after: datetime,
) -> datetime:
    after_utc = _require_aware(after, "after")
    spec = _parse_cron(expression)
    zone = _zone(timezone)
    start_date = after_utc.astimezone(zone).date()
    for day_offset in range(366 * 8 + 1):
        candidate_date = start_date + timedelta(days=day_offset)
        if not _date_matches(spec, candidate_date):
            continue
        for hour in sorted(spec.hours):
            for minute in sorted(spec.minutes):
                candidate = _wall_time_to_first_fold_utc(candidate_date, hour, minute, zone)
                if candidate is not None and candidate > after_utc:
                    return candidate
    raise InvalidScheduleRule("cron expression has no occurrence in the supported horizon")


def _previous_cron_occurrence(
    expression: str,
    timezone: str,
    *,
    at_or_before: datetime,
) -> datetime:
    boundary = _require_aware(at_or_before, "at_or_before")
    spec = _parse_cron(expression)
    zone = _zone(timezone)
    start_date = boundary.astimezone(zone).date()
    for day_offset in range(366 * 8 + 1):
        candidate_date = start_date - timedelta(days=day_offset)
        if not _date_matches(spec, candidate_date):
            continue
        for hour in sorted(spec.hours, reverse=True):
            for minute in sorted(spec.minutes, reverse=True):
                candidate = _wall_time_to_first_fold_utc(candidate_date, hour, minute, zone)
                if candidate is not None and candidate <= boundary:
                    return candidate
    raise InvalidScheduleRule("cron expression has no occurrence in the supported horizon")


def _positive_int(rule: dict[str, Any], field: str, *, maximum: int) -> int:
    value = rule.get(field)
    if isinstance(value, bool) or not isinstance(value, int) or value <= 0 or value > maximum:
        raise InvalidScheduleRule(f"{field} must be a bounded positive integer")
    return value


def _grace_seconds(rule: dict[str, Any]) -> int:
    value = rule.get("misfire_grace_seconds", 30)
    if isinstance(value, bool) or not isinstance(value, int) or value < 0 or value > 3600:
        raise InvalidScheduleRule("misfire_grace_seconds must be between 0 and 3600")
    return value


def _interval_plan(
    *,
    rule: dict[str, Any],
    next_fire_at: datetime,
    misfire_policy: Literal["skip", "run_once", "catch_up"],
    max_catch_up: int,
    now: datetime,
) -> DispatchPlan:
    seconds = _positive_int(rule, "seconds", maximum=31 * 24 * 60 * 60)
    grace = _grace_seconds(rule)
    elapsed = (now - next_fire_at).total_seconds()
    due_count = int(elapsed // seconds) + 1
    following = next_fire_at + timedelta(seconds=due_count * seconds)
    cutoff = now - timedelta(seconds=grace)
    missed_delta = (cutoff - next_fire_at).total_seconds()
    missed_count = min(due_count, max(0, math.ceil(missed_delta / seconds)))

    if misfire_policy == "skip":
        selected_indices = range(missed_count, due_count)
    elif misfire_policy == "run_once":
        selected_indices = range(due_count - 1, due_count)
    else:
        if max_catch_up < 1 or max_catch_up > 20:
            raise InvalidScheduleRule("max_catch_up must be between 1 and 20")
        selected_indices = range(max(0, due_count - max_catch_up), due_count)
    return DispatchPlan(
        fire_times=tuple(
            next_fire_at + timedelta(seconds=index * seconds) for index in selected_indices
        ),
        next_fire_at=following,
        completed=False,
        misfire_count=missed_count,
    )


def _cron_plan(
    *,
    rule: dict[str, Any],
    timezone: str,
    next_fire_at: datetime,
    misfire_policy: Literal["skip", "run_once", "catch_up"],
    max_catch_up: int,
    now: datetime,
) -> DispatchPlan:
    expression = rule.get("expression")
    if not isinstance(expression, str):
        raise InvalidScheduleRule("cron expression is required")
    grace = _grace_seconds(rule)
    following = next_cron_occurrence(expression, timezone, after=now)
    cutoff = now - timedelta(seconds=grace)
    missed = next_fire_at < cutoff

    if misfire_policy == "skip":
        lower_bound = max(next_fire_at, cutoff)
        selected: list[datetime] = []
        candidate = _previous_cron_occurrence(expression, timezone, at_or_before=now)
        while candidate >= lower_bound:
            selected.append(candidate)
            candidate = _previous_cron_occurrence(
                expression,
                timezone,
                at_or_before=candidate - timedelta(microseconds=1),
            )
        selected.reverse()
    elif misfire_policy == "run_once":
        latest = _previous_cron_occurrence(expression, timezone, at_or_before=now)
        selected = [latest] if latest >= next_fire_at else []
    else:
        if max_catch_up < 1 or max_catch_up > 20:
            raise InvalidScheduleRule("max_catch_up must be between 1 and 20")
        selected = []
        candidate = _previous_cron_occurrence(expression, timezone, at_or_before=now)
        while candidate >= next_fire_at and len(selected) < max_catch_up:
            selected.append(candidate)
            candidate = _previous_cron_occurrence(
                expression,
                timezone,
                at_or_before=candidate - timedelta(microseconds=1),
            )
        selected.reverse()
    return DispatchPlan(
        fire_times=tuple(selected),
        next_fire_at=following,
        completed=False,
        misfire_count=int(missed),
    )


def plan_due_occurrences(
    *,
    kind: Literal["one_off", "interval", "cron"],
    rule: dict[str, Any],
    timezone: str,
    next_fire_at: datetime,
    misfire_policy: Literal["skip", "run_once", "catch_up"],
    max_catch_up: int,
    now: datetime,
) -> DispatchPlan:
    next_fire_at_utc = _require_aware(next_fire_at, "next_fire_at")
    now_utc = _require_aware(now, "now")
    if next_fire_at_utc > now_utc:
        return DispatchPlan((), next_fire_at_utc, False, 0)
    if misfire_policy not in {"skip", "run_once", "catch_up"}:
        raise InvalidScheduleRule("unknown misfire policy")
    if kind == "one_off":
        grace = _grace_seconds(rule)
        missed = next_fire_at_utc < now_utc - timedelta(seconds=grace)
        fire_times = () if missed and misfire_policy == "skip" else (next_fire_at_utc,)
        return DispatchPlan(fire_times, None, True, int(missed))
    if kind == "interval":
        return _interval_plan(
            rule=rule,
            next_fire_at=next_fire_at_utc,
            misfire_policy=misfire_policy,
            max_catch_up=max_catch_up,
            now=now_utc,
        )
    if kind == "cron":
        return _cron_plan(
            rule=rule,
            timezone=timezone,
            next_fire_at=next_fire_at_utc,
            misfire_policy=misfire_policy,
            max_catch_up=max_catch_up,
            now=now_utc,
        )
    raise InvalidScheduleRule("unsupported schedule kind")

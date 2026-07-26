from __future__ import annotations

import pytest

from domain.task_states import (
    InvalidStateTransition,
    transition_attempt,
    transition_job,
    transition_task,
)


@pytest.mark.parametrize(
    ("current", "target", "event_type"),
    [
        ("queued", "running", "task.started"),
        ("queued", "cancelled", "task.cancelled"),
        ("running", "cancelling", "task.cancel_requested"),
        ("running", "succeeded", "task.succeeded"),
        ("running", "failed", "task.failed"),
        ("running", "unknown", "task.unknown"),
        ("cancelling", "cancelled", "task.cancelled"),
        ("unknown", "running", "task.reconciled"),
        ("unknown", "succeeded", "task.reconciled"),
        ("unknown", "failed", "task.reconciled"),
    ],
)
def test_task_transition_returns_matching_event_and_cas_version(
    current: str, target: str, event_type: str
) -> None:
    result = transition_task(current, target, resource_version=7)

    assert result.previous_status == current
    assert result.status == target
    assert result.previous_resource_version == 7
    assert result.resource_version == 8
    assert result.event_type == event_type


@pytest.mark.parametrize("terminal", ["cancelled", "succeeded", "failed"])
@pytest.mark.parametrize(
    "target", ["queued", "running", "cancelling", "cancelled", "succeeded", "failed", "unknown"]
)
def test_task_terminal_states_are_immutable(terminal: str, target: str) -> None:
    with pytest.raises(InvalidStateTransition):
        transition_task(terminal, target, resource_version=3)


@pytest.mark.parametrize(
    ("current", "target"),
    [
        ("created", "submitted"),
        ("submitted", "provider_running"),
        ("submitted", "downloading"),
        ("provider_running", "downloading"),
        ("downloading", "postprocessing"),
        ("postprocessing", "succeeded"),
        ("provider_running", "unknown"),
        ("unknown", "succeeded"),
    ],
)
def test_attempt_accepts_only_forward_or_reconciliation_transitions(
    current: str, target: str
) -> None:
    assert transition_attempt(current, target) == target


@pytest.mark.parametrize("terminal", ["succeeded", "failed", "cancelled"])
def test_attempt_terminal_states_are_immutable(terminal: str) -> None:
    with pytest.raises(InvalidStateTransition):
        transition_attempt(terminal, "unknown")


@pytest.mark.parametrize(
    ("current", "target"),
    [
        ("pending", "leased"),
        ("leased", "pending"),
        ("leased", "completed"),
        ("leased", "failed"),
        ("failed", "pending"),
    ],
)
def test_job_state_machine_supports_lease_release_and_explicit_replay(
    current: str, target: str
) -> None:
    assert transition_job(current, target) == target


@pytest.mark.parametrize(
    ("kind", "current", "target"),
    [
        ("task", "queued", "succeeded"),
        ("attempt", "created", "postprocessing"),
        ("job", "pending", "completed"),
        ("job", "completed", "pending"),
    ],
)
def test_illegal_transitions_fail_without_fallback(kind: str, current: str, target: str) -> None:
    function = {
        "task": lambda: transition_task(current, target, 1),
        "attempt": lambda: transition_attempt(current, target),
        "job": lambda: transition_job(current, target),
    }[kind]

    with pytest.raises(InvalidStateTransition):
        function()

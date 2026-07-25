from __future__ import annotations

from dataclasses import dataclass


class InvalidStateTransition(ValueError):
    def __init__(self, kind: str, current: str, target: str) -> None:
        super().__init__(f"invalid {kind} transition: {current} -> {target}")
        self.kind = kind
        self.current = current
        self.target = target


@dataclass(frozen=True, slots=True)
class TaskTransition:
    previous_status: str
    status: str
    previous_resource_version: int
    resource_version: int
    event_type: str


TASK_TRANSITIONS = {
    "queued": {"running", "cancelled"},
    "running": {"cancelling", "succeeded", "failed", "unknown"},
    "cancelling": {"cancelled", "failed", "unknown"},
    "unknown": {"running", "cancelled", "succeeded", "failed"},
    "cancelled": set(),
    "succeeded": set(),
    "failed": set(),
}

TASK_EVENTS = {
    ("queued", "running"): "task.started",
    ("queued", "cancelled"): "task.cancelled",
    ("running", "cancelling"): "task.cancel_requested",
    ("running", "succeeded"): "task.succeeded",
    ("running", "failed"): "task.failed",
    ("running", "unknown"): "task.unknown",
    ("cancelling", "cancelled"): "task.cancelled",
    ("cancelling", "failed"): "task.failed",
    ("cancelling", "unknown"): "task.unknown",
    ("unknown", "running"): "task.reconciled",
    ("unknown", "cancelled"): "task.reconciled",
    ("unknown", "succeeded"): "task.reconciled",
    ("unknown", "failed"): "task.reconciled",
}

ATTEMPT_TRANSITIONS = {
    "created": {"submitted", "failed", "cancelled", "unknown"},
    "submitted": {"provider_running", "downloading", "failed", "cancelled", "unknown"},
    "provider_running": {"downloading", "failed", "cancelled", "unknown"},
    "downloading": {"postprocessing", "failed", "cancelled", "unknown"},
    "postprocessing": {"succeeded", "failed", "cancelled", "unknown"},
    "unknown": {
        "provider_running",
        "downloading",
        "postprocessing",
        "succeeded",
        "failed",
        "cancelled",
    },
    "succeeded": set(),
    "failed": set(),
    "cancelled": set(),
}

JOB_TRANSITIONS = {
    "pending": {"leased"},
    "leased": {"pending", "completed", "failed"},
    "failed": {"pending"},
    "completed": set(),
}


def transition_task(current: str, target: str, resource_version: int) -> TaskTransition:
    if target not in TASK_TRANSITIONS.get(current, set()):
        raise InvalidStateTransition("task", current, target)
    return TaskTransition(
        previous_status=current,
        status=target,
        previous_resource_version=resource_version,
        resource_version=resource_version + 1,
        event_type=TASK_EVENTS[(current, target)],
    )


def transition_attempt(current: str, target: str) -> str:
    if target not in ATTEMPT_TRANSITIONS.get(current, set()):
        raise InvalidStateTransition("attempt", current, target)
    return target


def transition_job(current: str, target: str) -> str:
    if target not in JOB_TRANSITIONS.get(current, set()):
        raise InvalidStateTransition("job", current, target)
    return target

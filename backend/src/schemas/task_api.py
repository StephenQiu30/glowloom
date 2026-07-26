from __future__ import annotations

from schemas.common import (
    StrictContract,
    TaskAccepted,
    TaskError,
    TaskProgress,
    TaskResponse,
    TaskResultRef,
)
from schemas.tasks import (
    TaskAcceptedSnapshot,
    TaskSnapshot,
)


class TaskListResponse(StrictContract):
    items: tuple[TaskResponse, ...]


def task_response(value: TaskSnapshot) -> TaskResponse:
    error = None
    if value.error_code is not None:
        details = value.error or {}
        error = TaskError(
            code=value.error_code,
            retryable=bool(details.get("retryable", False)),
            summary=str(details.get("summary", value.error_code)),
        )
    progress = TaskProgress.model_validate(value.progress)
    return TaskResponse.model_validate(
        {
            "id": value.id,
            "type": value.task_type,
            "scope": {key: str(item) for key, item in value.scope.items()},
            "status": value.status,
            "progress": progress,
            "input_outdated": value.input_outdated,
            "current_attempt_id": value.current_attempt_id,
            "result_refs": tuple(
                TaskResultRef.model_validate(
                    {"output_type": item.output_type, "output_id": item.output_id}
                )
                for item in value.result_refs
            ),
            "error": error,
            "resource_version": value.resource_version,
            "created_at": value.created_at,
            "updated_at": value.updated_at,
            "finished_at": value.finished_at,
        }
    )


def task_accepted(value: TaskAcceptedSnapshot) -> TaskAccepted:
    return TaskAccepted(
        task_id=value.task_id,
        status="queued",
        resource_version=value.resource_version,
        status_url=value.status_url,
    )

from __future__ import annotations

from enum import IntEnum
from typing import Any

from schemas.common import Problem


class ApiErrorStatus(IntEnum):
    NOT_FOUND = 404
    CONFLICT = 409
    PRECONDITION_FAILED = 412
    UNPROCESSABLE_ENTITY = 422
    SERVICE_UNAVAILABLE = 503


def error_responses(
    *statuses: ApiErrorStatus,
) -> dict[int | str, dict[str, Any]]:
    return {status.value: {"model": Problem} for status in statuses}


RESOURCE_API_ERRORS = error_responses(
    ApiErrorStatus.NOT_FOUND,
    ApiErrorStatus.CONFLICT,
    ApiErrorStatus.PRECONDITION_FAILED,
    ApiErrorStatus.UNPROCESSABLE_ENTITY,
)

CANDIDATE_API_ERRORS = error_responses(
    ApiErrorStatus.NOT_FOUND,
    ApiErrorStatus.UNPROCESSABLE_ENTITY,
    ApiErrorStatus.SERVICE_UNAVAILABLE,
)

DELIVERY_API_ERRORS = error_responses(
    ApiErrorStatus.NOT_FOUND,
    ApiErrorStatus.UNPROCESSABLE_ENTITY,
    ApiErrorStatus.SERVICE_UNAVAILABLE,
)

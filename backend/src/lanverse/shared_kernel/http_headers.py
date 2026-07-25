from __future__ import annotations

import re

from lanverse.shared_kernel.http_errors import HttpProblem

IDEMPOTENCY_KEY_PATTERN = re.compile(r"^[A-Za-z0-9._:-]{8,128}$")
STRONG_ETAG_PATTERN = re.compile(r'^"([1-9][0-9]*)"$')


def validate_idempotency_key(value: str) -> str:
    if not IDEMPOTENCY_KEY_PATTERN.fullmatch(value):
        raise HttpProblem(
            status=422,
            title="Invalid idempotency key",
            code="INVALID_IDEMPOTENCY_KEY",
        )
    return value


def strong_etag(resource_version: int) -> str:
    if resource_version < 1:
        raise ValueError("resource_version must be positive")
    return f'"{resource_version}"'


def parse_if_match(value: str) -> int:
    match = STRONG_ETAG_PATTERN.fullmatch(value)
    if match is None:
        raise HttpProblem(
            status=422,
            title="Invalid If-Match header",
            code="INVALID_IF_MATCH",
        )
    return int(match.group(1))

from __future__ import annotations

import hashlib
import json
import unicodedata
from dataclasses import dataclass
from typing import Any
from uuid import UUID

import asyncpg  # type: ignore[import-untyped]
import rfc8785

from core.ids import new_id


class IdempotencyKeyReused(Exception):
    """A key already protects a different canonical request."""


class IdempotencyInProgress(Exception):
    """A previously committed multi-transaction operation remains pending."""


@dataclass(frozen=True, slots=True)
class StoredResponse:
    status: int
    reference: dict[str, Any]


def _normalize_strings(value: Any) -> Any:
    if isinstance(value, str):
        return unicodedata.normalize("NFC", value)
    if isinstance(value, list):
        return [_normalize_strings(item) for item in value]
    if isinstance(value, tuple):
        return [_normalize_strings(item) for item in value]
    if isinstance(value, dict):
        return {
            unicodedata.normalize("NFC", str(key)): _normalize_strings(item)
            for key, item in value.items()
        }
    return value


def canonical_request_hash(
    *, method: str, operation_id: str, path_parameters: dict[str, str], body: Any
) -> str:
    request = _normalize_strings(
        {
            "method": method.upper(),
            "operation_id": operation_id,
            "path_parameters": path_parameters,
            "body": body,
        }
    )
    return canonical_value_hash(request)


def canonical_value_hash(value: Any) -> str:
    return hashlib.sha256(rfc8785.dumps(_normalize_strings(value))).hexdigest()


class IdempotencyRepository:
    async def reserve(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        *,
        owner_module: str,
        operation_scope: str,
        idempotency_key: str,
        request_hash: str,
        request_id: UUID,
    ) -> StoredResponse | None:
        inserted = await connection.fetchval(
            """
            INSERT INTO idempotency_records(
                id, owner_module, operation_scope, idempotency_key, request_hash, request_id
            ) VALUES($1, $2, $3, $4, $5, $6)
            ON CONFLICT (operation_scope, idempotency_key) DO NOTHING
            RETURNING id
            """,
            new_id(),
            owner_module,
            operation_scope,
            idempotency_key,
            request_hash,
            request_id,
        )
        if inserted is not None:
            return None
        row = await connection.fetchrow(
            """
            SELECT request_hash, state, response_status, response_ref_json
            FROM idempotency_records
            WHERE operation_scope = $1 AND idempotency_key = $2
            FOR UPDATE
            """,
            operation_scope,
            idempotency_key,
        )
        if row is None or row["request_hash"] != request_hash:
            raise IdempotencyKeyReused
        if row["state"] != "completed":
            raise IdempotencyInProgress
        reference = row["response_ref_json"]
        if isinstance(reference, str):
            reference = json.loads(reference)
        return StoredResponse(status=row["response_status"], reference=reference)

    async def complete(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        *,
        operation_scope: str,
        idempotency_key: str,
        status: int,
        reference: dict[str, Any],
    ) -> None:
        await connection.execute(
            """
            UPDATE idempotency_records
            SET state = 'completed', response_status = $3,
                response_ref_json = $4::jsonb, updated_at = now(), completed_at = now()
            WHERE operation_scope = $1 AND idempotency_key = $2 AND state = 'pending'
            """,
            operation_scope,
            idempotency_key,
            status,
            json.dumps(reference, separators=(",", ":")),
        )

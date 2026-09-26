"""Redis-backed fake provider for workflow and failure-path verification."""

from collections.abc import Callable
from hashlib import sha256
from time import time_ns
from typing import Literal

from pydantic import BaseModel, ConfigDict, Field, ValidationError, model_validator
from redis.asyncio import Redis
from redis.exceptions import RedisError


class SubmitRequest(BaseModel):
    operation_id: str = Field(min_length=1)
    request_key: str = Field(min_length=1)
    provider_model_id: str = Field(min_length=1)
    capability: str = Field(min_length=1)
    mode: str = Field(min_length=1)
    params: dict[str, object]
    inputs: list[dict[str, object]]
    output_count: int = Field(ge=1)


class TaskRef(BaseModel):
    request_key: str | None = None
    provider_task_id: str | None = None

    @model_validator(mode="after")
    def exactly_one_reference(self) -> "TaskRef":
        if bool(self.request_key) == bool(self.provider_task_id):
            raise ValueError("provide either request_key or provider_task_id")
        return self


class SubmitResult(BaseModel):
    outcome: Literal["accepted", "rejected", "not_submitted", "unknown"]
    provider_task_id: str | None = None
    error_code: str | None = None


class QueryResult(BaseModel):
    state: Literal["pending", "running", "succeeded", "failed", "not_found"]
    result_urls: list[str] = Field(default_factory=list)
    error_code: str | None = None


class CancelResult(BaseModel):
    cancelled: bool
    error_code: str | None = None


class MockOptions(BaseModel):
    model_config = ConfigDict(extra="ignore")

    mock_outcome: Literal["accepted", "rejected", "not_submitted", "unknown"] = "accepted"
    mock_delay_ms: int = Field(default=0, ge=0)
    mock_result_urls: list[str] = Field(default_factory=list)
    mock_fail: bool = False
    mock_result_expire_ms: int | None = Field(default=None, ge=0)


class TaskRecord(BaseModel):
    operation_id: str
    request_key: str
    ready_at_ms: int
    expires_at_ms: int | None
    result_urls: list[str]
    failed: bool
    cancelled: bool = False


def current_time_ms() -> int:
    return time_ns() // 1_000_000


class MockProvider:
    """Simulate provider task state without writing Lanverse business records."""

    key = "mock"

    def __init__(
        self,
        store: Redis,
        *,
        now_ms: Callable[[], int] = current_time_ms,
        task_ttl_seconds: int = 86_400,
        key_prefix: str = "lanverse:mock-provider",
    ) -> None:
        if task_ttl_seconds < 1:
            raise ValueError("task_ttl_seconds must be positive")
        self._store = store
        self._now_ms = now_ms
        self._task_ttl_seconds = task_ttl_seconds
        self._key_prefix = key_prefix

    def _task_id(self, request_key: str) -> str:
        return f"mock-{sha256(request_key.encode()).hexdigest()[:32]}"

    def _store_key(self, task_id: str) -> str:
        return f"{self._key_prefix}:{task_id}"

    def _reference_key(self, ref: TaskRef) -> str:
        task_id = ref.provider_task_id or self._task_id(ref.request_key or "")
        return self._store_key(task_id)

    async def _load(self, ref: TaskRef) -> TaskRecord | None:
        raw = await self._store.get(self._reference_key(ref))
        return TaskRecord.model_validate_json(raw) if raw is not None else None

    def _replay_result(self, raw: bytes | str, req: SubmitRequest, task_id: str) -> SubmitResult:
        previous = TaskRecord.model_validate_json(raw)
        if previous.operation_id != req.operation_id or previous.request_key != req.request_key:
            return SubmitResult(outcome="rejected", error_code="request_key_conflict")
        return SubmitResult(outcome="accepted", provider_task_id=task_id)

    async def submit(self, req: SubmitRequest) -> SubmitResult:
        task_id = self._task_id(req.request_key)
        key = self._store_key(task_id)
        try:
            existing = await self._store.get(key)
        except RedisError:
            return SubmitResult(outcome="not_submitted", error_code="mock_store_unavailable")
        if existing is not None:
            return self._replay_result(existing, req, task_id)

        try:
            options = MockOptions.model_validate(req.params)
        except ValidationError:
            return SubmitResult(outcome="rejected", error_code="invalid_mock_parameters")

        if options.mock_outcome in {"rejected", "not_submitted"}:
            return SubmitResult(outcome=options.mock_outcome)

        now = self._now_ms()
        record = TaskRecord(
            operation_id=req.operation_id,
            request_key=req.request_key,
            ready_at_ms=now + options.mock_delay_ms,
            expires_at_ms=(
                now + options.mock_delay_ms + options.mock_result_expire_ms
                if options.mock_result_expire_ms is not None
                else None
            ),
            result_urls=options.mock_result_urls,
            failed=options.mock_fail,
        )
        try:
            created = await self._store.set(
                key, record.model_dump_json(), ex=self._task_ttl_seconds, nx=True
            )
        except RedisError:
            return SubmitResult(outcome="unknown", error_code="mock_store_result_unknown")
        if not created:
            try:
                existing = await self._store.get(key)
            except RedisError:
                return SubmitResult(outcome="unknown", error_code="mock_store_result_unknown")
            if existing is None:
                return SubmitResult(outcome="unknown", error_code="mock_task_expired")
            return self._replay_result(existing, req, task_id)

        if options.mock_outcome == "unknown":
            return SubmitResult(outcome="unknown")
        return SubmitResult(outcome="accepted", provider_task_id=task_id)

    async def query(self, ref: TaskRef) -> QueryResult:
        record = await self._load(ref)
        if record is None:
            return QueryResult(state="not_found")
        now = self._now_ms()
        if record.expires_at_ms is not None and now >= record.expires_at_ms:
            return QueryResult(state="not_found")
        if record.cancelled:
            return QueryResult(state="failed", error_code="cancelled")
        if now < record.ready_at_ms:
            return QueryResult(state="pending")
        if record.failed:
            return QueryResult(state="failed", error_code="mock_failure")
        return QueryResult(state="succeeded", result_urls=record.result_urls)

    async def cancel(self, ref: TaskRef) -> CancelResult:
        key = self._reference_key(ref)
        record = await self._load(ref)
        if record is None:
            return CancelResult(cancelled=False, error_code="not_found")
        if record.cancelled:
            return CancelResult(cancelled=True)
        if self._now_ms() >= record.ready_at_ms:
            return CancelResult(cancelled=False, error_code="already_finished")
        record.cancelled = True
        saved = await self._store.set(key, record.model_dump_json(), xx=True, keepttl=True)
        return CancelResult(cancelled=bool(saved), error_code=None if saved else "not_found")

"""Creation application use cases used by the FastAPI routes."""

from __future__ import annotations

from datetime import timedelta
from typing import Any

from temporalio.client import Client
from temporalio.service import RPCError

from app.creation.contract import Command, canonical_uuid, decode_object
from app.creation.execution import ExecutionConflict, ExecutionStore
from app.creation.manifest import ManifestConflict, read_manifest
from app.creation.repository import CommandConflict, Repository
from app.creation.temporal import TemporalStarter


class CreationRequestError(ValueError):
    """A safe, client-visible validation failure from a Creation use case."""

    def __init__(self, detail: str, status_code: int = 422) -> None:
        super().__init__(detail)
        self.detail = detail
        self.status_code = status_code


class CreationService:
    """Application service for durable command and execution operations."""

    def __init__(
        self,
        repository: Repository,
        task_queue: str,
        temporal: Client | None = None,
    ) -> None:
        self.repository = repository
        self.task_queue = task_queue
        self.temporal = temporal
        self.execution = ExecutionStore(repository)

    async def accept(self, raw: bytes) -> Any:
        try:
            command = Command.model_validate(decode_object(raw))
        except ValueError as error:
            raise CreationRequestError("creation_command_invalid") from error
        try:
            return await self.repository.accept(command, self.task_queue)
        except CommandConflict as error:
            raise CreationRequestError("creation_command_conflict", 409) from error

    async def lookup(self, command_id: str, raw: bytes) -> Any:
        self._validate_empty_lookup(command_id, raw)
        receipt = await self.repository.get(command_id)
        if receipt is None:
            raise CreationRequestError("creation_command_not_found", 404)
        return receipt

    async def execution_snapshot(self, command_id: str, raw: bytes) -> dict[str, Any]:
        identity = await self._execution_identity(command_id, raw)
        try:
            saved = await self.execution.snapshot(command_id)
        except ExecutionConflict as error:
            if str(error) == "execution_policy_missing":
                raise CreationRequestError("creation_execution_not_started", 404) from error
            raise
        return {"schema": "creation-execution-production", **identity, **saved}

    async def draft(self, command_id: str, draft_id: str, raw: bytes) -> dict[str, Any]:
        identity = await self._execution_identity(command_id, raw)
        try:
            canonical_uuid(draft_id)
        except ValueError as error:
            raise CreationRequestError("creation_draft_invalid") from error
        saved = await self.execution.draft_envelope(command_id, draft_id)
        if saved is None:
            raise CreationRequestError("creation_draft_not_found", 404)
        return {"schema": "creation-draft-production", **identity, **saved}

    async def attempts(self, command_id: str, step_id: str, raw: bytes) -> dict[str, Any]:
        self._validate_empty_lookup(command_id, raw)
        try:
            canonical_uuid(step_id)
        except ValueError as error:
            raise CreationRequestError("creation_attempt_lookup_invalid") from error
        command = await self.repository.command(command_id)
        if command is None:
            raise CreationRequestError("creation_command_not_found", 404)
        history = await self.execution.attempt_history(command_id, step_id)
        if history is None:
            raise CreationRequestError("creation_step_not_found", 404)
        return {
            "schema": "creation-attempt-history-production",
            "command_id": command_id,
            "run_id": command.run_id,
            **history,
        }

    async def manifest(self, command_id: str, raw: bytes) -> dict[str, Any]:
        self._validate_empty_lookup(command_id, raw)
        try:
            async with await self.repository.connect() as conn:
                saved = await read_manifest(conn, command_id)
        except ManifestConflict as error:
            raise CreationRequestError("creation_manifest_conflict", 409) from error
        if saved is None:
            raise CreationRequestError("creation_command_not_found", 404)
        return saved

    async def resume(self, command_id: str, raw: bytes) -> dict[str, Any]:
        try:
            canonical_uuid(command_id)
            payload = decode_object(raw)
            if set(payload) != {"payload_hash"}:
                raise ValueError("invalid resume input")
        except ValueError as error:
            raise CreationRequestError("creation_resume_invalid") from error
        command = await self.repository.command(command_id)
        if command is None:
            raise CreationRequestError("creation_command_not_found", 404)
        if payload["payload_hash"] != command.payload_hash:
            raise CreationRequestError("creation_command_conflict", 409)
        saved = await self.execution.snapshot(command_id)
        if saved["status"] in {"running", "waiting_review", "completed"}:
            return {
                "schema": "creation-resume-production",
                "command_id": command_id,
                "run_id": command.run_id,
                "status": "resume_requested",
            }
        if saved["status"] != "blocked" or not saved["can_resume"]:
            raise CreationRequestError("creation_resume_not_allowed", 409)
        if self.temporal is None:
            raise CreationRequestError("creation_runtime_unavailable", 503)
        handle = self.temporal.get_workflow_handle(command.workflow_id)
        try:
            description = await handle.describe(rpc_timeout=timedelta(seconds=5))
            if description.workflow_type != command.flow_type:
                raise CreationRequestError("creation_workflow_identity_conflict", 409)
            for key, value in TemporalStarter.memo(command).items():
                if await description.memo_value(key, default=None) != value:
                    raise CreationRequestError("creation_workflow_identity_conflict", 409)
            await handle.signal("resume", rpc_timeout=timedelta(seconds=5))
        except RPCError as error:
            raise CreationRequestError("creation_runtime_unavailable", 503) from error
        return {
            "schema": "creation-resume-production",
            "command_id": command_id,
            "run_id": command.run_id,
            "status": "resume_requested",
        }

    async def ready(self) -> None:
        await self.repository.ready()

    async def _execution_identity(self, command_id: str, raw: bytes) -> dict[str, str]:
        self._validate_empty_lookup(command_id, raw)
        command = await self.repository.command(command_id)
        if command is None:
            raise CreationRequestError("creation_command_not_found", 404)
        return {
            "command_id": command_id,
            "run_id": command.run_id,
            "payload_hash": command.payload_hash,
            "source_revision_id": command.source.revision_id,
        }

    @staticmethod
    def _validate_empty_lookup(command_id: str, raw: bytes) -> None:
        try:
            canonical_uuid(command_id)
            if raw:
                raise ValueError("lookup body must be empty")
        except ValueError as error:
            raise CreationRequestError("creation_command_invalid") from error

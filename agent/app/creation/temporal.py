from __future__ import annotations

from datetime import timedelta

from temporalio.api.workflowservice.v1 import DescribeNamespaceRequest
from temporalio.client import Client
from temporalio.common import WorkflowIDReusePolicy
from temporalio.exceptions import WorkflowAlreadyStartedError
from temporalio.service import RPCError, RPCStatusCode

from app.creation.contract import Command


class WorkflowIdentityConflict(RuntimeError):
    pass


class StartPolicyViolation(RuntimeError):
    pass


class TemporalStarter:
    def __init__(self, client: Client) -> None:
        self.client = client

    @staticmethod
    def memo(command: Command) -> dict[str, str]:
        return {
            "creation_command_id": command.command_id,
            "creation_run_id": command.run_id,
            "creation_payload_hash": command.payload_hash,
        }

    async def ensure_started(
        self, command: Command, task_queue: str, *, allow_start: bool = True
    ) -> str:
        existing = await self._lookup(command, task_queue)
        if existing is not None:
            return existing
        if not allow_start:
            raise StartPolicyViolation("automatic start window has expired")
        namespace = await self.client.workflow_service.describe_namespace(
            DescribeNamespaceRequest(namespace=self.client.namespace), timeout=timedelta(seconds=5)
        )
        if namespace.config.workflow_execution_retention_ttl.seconds < 86400:
            raise StartPolicyViolation("creation requires at least one day of Temporal retention")
        try:
            await self.client.start_workflow(
                command.flow_type,
                command.model_dump(mode="json", by_alias=True),
                id=command.workflow_id,
                task_queue=task_queue,
                id_reuse_policy=WorkflowIDReusePolicy.REJECT_DUPLICATE,
                memo=self.memo(command),
                rpc_timeout=timedelta(seconds=5),
            )
        except WorkflowAlreadyStartedError:
            # Another dispatcher may have started the same identity after our lookup.
            pass
        existing = await self._lookup(command, task_queue)
        if existing is None:
            raise RuntimeError("started workflow outcome is unknown")
        return existing

    async def _lookup(self, command: Command, task_queue: str) -> str | None:
        try:
            description = await self.client.get_workflow_handle(command.workflow_id).describe(
                rpc_timeout=timedelta(seconds=5)
            )
        except RPCError as error:
            if error.status == RPCStatusCode.NOT_FOUND:
                return None
            raise
        if description.workflow_type != command.flow_type or description.task_queue != task_queue:
            raise WorkflowIdentityConflict("workflow route differs from the accepted command")
        for key, value in self.memo(command).items():
            if await description.memo_value(key, default=None) != value:
                raise WorkflowIdentityConflict(
                    "workflow identity differs from the accepted command"
                )
        return description.run_id

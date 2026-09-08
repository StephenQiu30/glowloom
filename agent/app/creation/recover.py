"""Operator-only recovery: python -m app.creation.recover --help.

The operator must reconcile the original model process before authorizing a retry.
An uncertain reset response is retried with the SAME request ID and arguments.
"""

import argparse
import asyncio
import json
from datetime import timedelta

from temporalio.api.common.v1 import WorkflowExecution
from temporalio.api.enums.v1 import EventType, ResetReapplyType
from temporalio.api.workflowservice.v1 import ResetWorkflowExecutionRequest
from temporalio.client import Client, WorkflowExecutionStatus

from app.core.config import Settings
from app.creation.execution import ExecutionConflict, ExecutionStore
from app.creation.recovery import RecoveryStore
from app.creation.repository import Repository
from app.creation.temporal import TemporalStarter


async def recover(
    repository: Repository,
    client: Client,
    *,
    request_id: str,
    command_id: str,
    step_id: str,
    attempt_id: str,
    reason: str,
) -> str:
    await repository.ready()
    command = await repository.command(command_id)
    if command is None:
        raise ExecutionConflict("creation_command_missing")
    store = RecoveryStore(repository)
    previous = await store.get(request_id)
    if previous:
        if (
            str(previous["command_id"]),
            str(previous["step_id"]),
            str(previous["previous_attempt_id"]),
            previous["reason"],
        ) != (command_id, step_id, attempt_id, reason):
            raise ExecutionConflict("recovery_request_conflict")
        if previous["reset_run_id"]:
            return str(previous["reset_run_id"])
        run_id, event_id = str(previous["temporal_run_id"]), previous["reset_event_id"]
    else:
        handle = client.get_workflow_handle(command.workflow_id)
        description = await handle.describe(rpc_timeout=timedelta(seconds=10))
        if (
            description.status != WorkflowExecutionStatus.RUNNING
            or description.workflow_type != command.flow_type
            or description.raw_description.pending_activities
        ):
            raise ExecutionConflict("recovery_workflow_not_quiescent")
        for key, expected in TemporalStarter.memo(command).items():
            if await description.memo_value(key, default=None) != expected:
                raise ExecutionConflict("recovery_workflow_identity_conflict")
        snapshot = await ExecutionStore(repository).snapshot(command_id)
        if snapshot["status"] != "blocked":
            raise ExecutionConflict("recovery_target_invalid")
        run_id = description.run_id
        history = await client.get_workflow_handle(
            command.workflow_id, run_id=run_id
        ).fetch_history()
        event_id = next(
            event.event_id
            for event in history.events
            if event.event_type == EventType.EVENT_TYPE_WORKFLOW_TASK_COMPLETED
        )
        await store.authorize(request_id, command_id, step_id, attempt_id, run_id, event_id, reason)
    response = await client.workflow_service.reset_workflow_execution(
        ResetWorkflowExecutionRequest(
            namespace=client.namespace,
            workflow_execution=WorkflowExecution(workflow_id=command.workflow_id, run_id=run_id),
            reason="Lanverse authorized recovery " + request_id,
            workflow_task_finish_event_id=event_id,
            request_id=request_id,
            reset_reapply_type=ResetReapplyType.RESET_REAPPLY_TYPE_NONE,
        ),
        timeout=timedelta(seconds=15),
    )
    await store.record_reset(request_id, response.run_id)
    return response.run_id


async def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("request-id", "command-id", "step-id", "attempt-id", "reason"):
        parser.add_argument("--" + name, required=True)
    parser.add_argument(
        "--confirm-no-active-invocation",
        action="store_true",
        required=True,
        help="Confirm original model process ended and no recoverable output remains",
    )
    args = parser.parse_args()
    settings = Settings.from_environment()
    client = await Client.connect(
        settings.temporal_address, namespace=settings.temporal_namespace, tls=settings.temporal_tls
    )
    run_id = await recover(
        Repository(settings.database_url),
        client,
        request_id=args.request_id,
        command_id=args.command_id,
        step_id=args.step_id,
        attempt_id=args.attempt_id,
        reason=args.reason,
    )
    print(
        json.dumps(
            {
                "request_id": args.request_id,
                "workflow_run_id": run_id,
                "status": "recovery_dispatched",
            }
        )
    )


if __name__ == "__main__":
    asyncio.run(main())

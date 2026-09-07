from __future__ import annotations

import asyncio
import logging
from typing import Protocol

from app.creation.contract import Command
from app.creation.repository import Repository
from app.creation.temporal import StartPolicyViolation, WorkflowIdentityConflict

logger = logging.getLogger(__name__)


class WorkflowStarter(Protocol):
    async def ensure_started(
        self, command: Command, task_queue: str, *, allow_start: bool
    ) -> str: ...


class Dispatcher:
    def __init__(self, repository: Repository, starter: WorkflowStarter) -> None:
        self.repository = repository
        self.starter = starter

    async def once(self) -> bool:
        lease = await self.repository.claim()
        if lease is None:
            return False
        try:
            async with asyncio.timeout(15):
                run_id = await self.starter.ensure_started(
                    lease.command, lease.task_queue, allow_start=lease.start_allowed
                )
        except StartPolicyViolation:
            await self.repository.fail(lease, "workflow_start_policy_blocked", blocked=True)
        except WorkflowIdentityConflict:
            await self.repository.fail(lease, "workflow_identity_conflict", blocked=True)
        except Exception:
            # Cancellation propagates; its lease expires naturally. Never log remote diagnostics.
            await self.repository.fail(lease, "temporal_outcome_unknown")
        else:
            await self.repository.finish(lease, temporal_run_id=run_id)
        return True

    async def run(self) -> None:
        while True:
            try:
                if await self.once():
                    continue
            except Exception:
                logger.error("creation_dispatcher_iteration_failed")
            await asyncio.sleep(1)

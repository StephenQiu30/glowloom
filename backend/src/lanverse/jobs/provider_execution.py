from __future__ import annotations

from dataclasses import dataclass
from typing import Literal, Protocol

from lanverse.infrastructure.database.pool import DatabasePool
from lanverse.jobs.dispatch import JobContext
from lanverse.modules.production_jobs.infrastructure.executions import TaskExecutionStore


class InjectedFault(RuntimeError):
    pass


class FaultInjector:
    def hit(self, point: str) -> None:
        del point


@dataclass(frozen=True, slots=True)
class ProviderOutcome:
    state: Literal["not_found", "unknown", "succeeded"]
    provider_request_id: str | None


class RecoverableProvider(Protocol):
    async def submit(self, request_key: str, prompt: str) -> ProviderOutcome: ...

    async def reconcile(self, request_key: str) -> ProviderOutcome: ...

    async def download(self, outcome: ProviderOutcome) -> None: ...


class ProviderExecutionHandler:
    def __init__(
        self,
        database: DatabasePool,
        *,
        provider: RecoverableProvider,
        fault: FaultInjector,
    ) -> None:
        self._store = TaskExecutionStore(database)
        self._provider = provider
        self._fault = fault

    async def handle(self, context: JobContext) -> None:
        plan = await self._store.prepare(context.payload)
        if plan.skip:
            return
        outcome: ProviderOutcome
        if plan.reconcile_first:
            outcome = await self._provider.reconcile(plan.provider_request_key)
            if outcome.state == "unknown":
                await self._store.mark_unknown(plan)
                return
            if outcome.state == "not_found":
                self._fault.hit("before_provider_submit")
                outcome = await self._provider.submit(plan.provider_request_key, plan.prompt)
        else:
            self._fault.hit("before_provider_submit")
            outcome = await self._provider.submit(plan.provider_request_key, plan.prompt)
        self._fault.hit("after_provider_accept")
        if outcome.state == "unknown":
            await self._store.mark_unknown(plan)
            return
        if outcome.state != "succeeded" or outcome.provider_request_id is None:
            raise RuntimeError("provider returned an invalid terminal outcome")
        await self._store.record_provider_success(plan, outcome.provider_request_id)
        self._fault.hit("before_download")
        await self._provider.download(outcome)
        self._fault.hit("after_download")
        self._fault.hit("before_registration")
        await self._store.mark_succeeded(plan)

"""Durable ordering only. All HTTP, database and inference work lives in Activities."""

from __future__ import annotations

from datetime import timedelta
from typing import Any

from temporalio import workflow
from temporalio.common import RetryPolicy
from temporalio.exceptions import ActivityError, ApplicationError

FLOW_TYPE = "lanverse.creation.text-storyboard.production"


@workflow.defn(name=FLOW_TYPE)
class TextStoryboardWorkflow:
    def __init__(self) -> None:
        self.woken = False
        self.command: dict[str, Any] = {}
        self.resumed = False

    @workflow.signal(name="review_changed")
    def review_changed(self) -> None:
        # A signal is only a hint. The platform's stored Owner Effect decides the gate.
        self.woken = True

    @workflow.signal(name="resume")
    def resume(self) -> None:
        self.resumed = True

    async def _activity(self, name: str, request: dict[str, Any], *, model: bool = False) -> Any:
        return await workflow.execute_activity(
            name,
            request,
            start_to_close_timeout=timedelta(seconds=1200 if model else 45),
            retry_policy=RetryPolicy(
                maximum_attempts=0 if name == "creation.progress" else 3,
                non_retryable_error_types=["ExecutionConflict", "ValueError"],
            ),
        )

    async def _gate(self, command_id: str, stage: str) -> dict[str, Any]:
        polls = 0
        while True:
            # Restart only durable orchestration history. Activities recover the same immutable
            # drafts and recheck platform receipts; this never renews the model call budget.
            if polls >= 500:
                workflow.continue_as_new(self.command)
            self.woken = False
            state = await self._activity(
                "creation.resolve_gate", {"command_id": command_id, "stage": stage}
            )
            if state["status"] == "accepted":
                return state
            if state["status"] == "rejected":
                raise ApplicationError("review_rejected", non_retryable=True)
            try:
                await workflow.wait_condition(lambda: self.woken, timeout=timedelta(seconds=30))
            except TimeoutError:
                pass
            polls += 1

    @workflow.run
    async def run(self, command: dict[str, Any]) -> dict[str, str]:
        self.command = command
        command_id = command["command_id"]
        stage = "map_manuscript"
        try:
            mapped = await self._activity(
                "creation.invoke_text", {"command_id": command_id, "stage": stage}, model=True
            )
            await self._gate(command_id, stage)
            stage = "analyze_episode"
            for episode_key in mapped["episodes"]:
                await self._activity(
                    "creation.invoke_text",
                    {"command_id": command_id, "stage": stage, "episode_key": episode_key},
                    model=True,
                )
            structure = await self._gate(command_id, stage)
            stage = "build_world"
            await self._activity(
                "creation.invoke_text", {"command_id": command_id, "stage": stage}, model=True
            )
            await self._gate(command_id, stage)
            stage = "direct_scene"
            for scene in structure["selected_scenes"]:
                await self._activity(
                    "creation.invoke_text",
                    {"command_id": command_id, "stage": stage, **scene},
                    model=True,
                )
            await self._gate(command_id, stage)
        except (ActivityError, ApplicationError) as error:
            cause = error.cause if isinstance(error, ActivityError) else error
            code = str(cause).split(":")[-1].strip() if cause else "activity_failed"
            safe_codes = {
                "review_rejected",
                "harness_response_unknown",
                "harness_result_invalid",
                "invocation_outcome_unknown",
                "invocation_budget_exhausted",
                "scene_not_selected",
                "selected_scenes_invalid",
                "upstream_gate_pending",
                "platform_unavailable",
                "platform_source_mismatch",
                "platform_source_invalid",
                "gate_response_invalid",
                "gate_receipt_mismatch",
                "execution_policy_conflict",
                "execution_input_mismatch",
                "step_input_conflict",
                "invocation_in_progress",
                "attempt_fence_lost",
                "persisted_draft_drift",
                "internal_response_too_large",
                "execution_policy_missing",
                "creation_command_missing",
                "upstream_draft_missing",
                "upstream_map_conflict",
                "upstream_world_conflict",
                "gate_draft_missing",
                "result_binding_mismatch",
                "draft_result_conflict",
                "result_checks_mismatch",
            }
            safe_codes.update(f"internal_http_{status}" for status in range(100, 600))
            recoverable = {
                "platform_unavailable",
                "internal_http_401",
                "internal_http_403",
                "internal_http_429",
                "internal_http_500",
                "internal_http_502",
                "internal_http_503",
                "internal_http_504",
            }
            safe_codes.update(recoverable)
            if code not in safe_codes:
                code = "activity_failed"
            can_resume = code in recoverable
            self.resumed = False
            await self._activity(
                "creation.progress",
                {
                    "command_id": command_id,
                    "stage": stage,
                    "status": "rejected" if code == "review_rejected" else "blocked",
                    "last_error": code,
                    "can_resume": can_resume,
                },
            )
            if code == "review_rejected":
                return {"run_id": command_id, "status": "rejected"}
            # Technical failures resume only on a signed, authorized request for this run.
            # Unknown inference or changed frozen input cannot be retried through this signal.
            await workflow.wait_condition(lambda: self.resumed and can_resume)
            workflow.continue_as_new(self.command)
        await self._activity(
            "creation.progress", {"command_id": command_id, "stage": stage, "status": "completed"}
        )
        return {"run_id": command_id, "status": "completed"}

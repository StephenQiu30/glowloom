from __future__ import annotations

import asyncio
from typing import Any

from temporalio import activity
from temporalio.exceptions import ApplicationError

from app.creation.contract import Command
from app.creation.execution import ExecutionConflict, ExecutionStore, InvocationLease
from app.creation.platform import HarnessClient, PlatformClient
from app.protocol.canonical import canonical_hash
from app.text_contract.failure import HarnessFailed
from app.text_contract.schemas import EpisodeAnalysis, EpisodeMap, WorldBook
from app.text_contract.task import Stage, TextTask


class CreationActivities:
    def __init__(
        self,
        store: ExecutionStore,
        platform: PlatformClient,
        harness: HarnessClient,
        release_hash: str,
        call_limit: int,
        invocation_timeout_seconds: int = 300,
    ) -> None:
        self.store, self.platform, self.harness = store, platform, harness
        self.release_hash, self.call_limit = release_hash, call_limit
        self.invocation_timeout_seconds = invocation_timeout_seconds

    async def _command(self, command_id: str) -> Command:
        command = await self.store.repository.command(command_id)
        if command is None:
            raise ExecutionConflict("creation_command_missing")
        return command

    async def _outputs(self, command_id: str, stage: Stage) -> list[dict[str, Any]]:
        snapshot = await self.store.snapshot(command_id)
        return [item for item in snapshot["outputs"] if item["step_key"].split("/")[0] == stage]

    async def _approved(self, command: Command, stage: Stage) -> list[dict[str, Any]]:
        outputs = await self._outputs(command.command_id, stage)
        if not outputs:
            raise ExecutionConflict("upstream_draft_missing")
        gate = await self.platform.gate(command, stage, outputs)
        if gate["status"] != "accepted":
            raise ExecutionConflict("upstream_gate_pending")
        drafts: list[dict[str, Any]] = []
        for output in outputs:
            result = await self.store.draft(command.command_id, output["draft_id"])
            if result is None:
                raise ExecutionConflict("upstream_draft_missing")
            drafts.append(result["candidate"])
        return drafts

    @activity.defn(name="creation.invoke_text")
    async def invoke(self, request: dict[str, Any]) -> dict[str, Any]:
        command = await self._command(request["command_id"])
        await self.store.freeze(command.command_id, self.release_hash, self.call_limit)
        source = await self.platform.source(command)
        stage: Stage = request["stage"]
        values: dict[str, Any] = {}
        if stage != "map_manuscript":
            maps = await self._approved(command, "map_manuscript")
            if len(maps) != 1:
                raise ExecutionConflict("upstream_map_conflict")
            values["episode_map"] = EpisodeMap.model_validate(maps[0])
        if stage in {"build_world", "direct_scene"}:
            values["analyses"] = [
                EpisodeAnalysis.model_validate(item)
                for item in await self._approved(command, "analyze_episode")
            ]
        if stage == "direct_scene":
            worlds = await self._approved(command, "build_world")
            if len(worlds) != 1:
                raise ExecutionConflict("upstream_world_conflict")
            values["world"] = WorldBook.model_validate(worlds[0])
            structure = await self.platform.gate(
                command,
                "analyze_episode",
                await self._outputs(command.command_id, "analyze_episode"),
            )
            if {
                "episode_key": request.get("episode_key"),
                "scene_key": request.get("scene_key"),
            } not in structure.get("selected_scenes", []):
                raise ExecutionConflict("scene_not_selected")
        scope = "/".join(
            str(request[key]) for key in ("stage", "episode_key", "scene_key") if request.get(key)
        )
        invocation_id = f"{command.run_id}/{scope}"
        if len(invocation_id) > 128:
            invocation_id = f"{command.run_id}/{canonical_hash({'scope': scope})}"
        task = TextTask(
            invocation_id=invocation_id,
            stage=stage,
            source=source,
            release_hash=self.release_hash,
            timeout_seconds=self.invocation_timeout_seconds,
            episode_key=request.get("episode_key"),
            scene_key=request.get("scene_key"),
            **values,
        )
        await self.store.progress(command.command_id, "running", stage)
        lease = await self.store.reserve(command.command_id, task)
        if isinstance(lease, InvocationLease):
            try:
                raw = await self.harness.invoke(task)
            except HarnessFailed as error:
                if not await self.store.failed(lease, error.failure):
                    raise ApplicationError("attempt_fence_lost", non_retryable=True) from None
                raise ApplicationError(error.failure.error_code, non_retryable=True) from None
            except asyncio.CancelledError:
                await self.store.unknown(lease, "attempt_cancelled")
                raise
            except Exception as error:
                await self.store.unknown(lease, "harness_response_unknown")
                raise ApplicationError("harness_response_unknown", non_retryable=True) from error
            try:
                await self.store.finish(lease, raw)
            except ValueError:
                await self.store.unknown(lease, "harness_result_invalid")
                raise ApplicationError("harness_result_invalid", non_retryable=True) from None
        outputs = await self._outputs(command.command_id, stage)
        output = next(item for item in outputs if item["step_key"] == scope)
        result = await self.store.draft(command.command_id, output["draft_id"])
        assert result is not None
        response: dict[str, Any] = {"output": output}
        if stage == "map_manuscript":
            episodes = EpisodeMap.model_validate(result["candidate"]).episodes
            # Reserve room for the world and at least one selected scene before starting episodes.
            if 1 + len(episodes) + 1 + 1 > self.call_limit:
                raise ExecutionConflict("invocation_budget_exhausted")
            response["episodes"] = [item.key for item in episodes]
        return response

    @activity.defn(name="creation.resolve_gate")
    async def gate(self, request: dict[str, Any]) -> dict[str, Any]:
        command = await self._command(request["command_id"])
        stage: Stage = request["stage"]
        outputs = await self._outputs(command.command_id, stage)
        if not outputs:
            raise ExecutionConflict("gate_draft_missing")
        await self.store.progress(command.command_id, "waiting_review", stage)
        response = await self.platform.gate(command, stage, outputs)
        if response["status"] == "accepted" and stage == "analyze_episode":
            selected = response.get("selected_scenes", [])
            allowed: set[tuple[str, str]] = set()
            for output in outputs:
                result = await self.store.draft(command.command_id, output["draft_id"])
                assert result is not None
                analysis = EpisodeAnalysis.model_validate(result["candidate"])
                allowed.update((analysis.episode_key, scene.key) for scene in analysis.scenes)
            keys = [(item["episode_key"], item["scene_key"]) for item in selected]
            if not keys or len(keys) != len(set(keys)) or not set(keys) <= allowed:
                raise ExecutionConflict("selected_scenes_invalid")
            snapshot = await self.store.snapshot(command.command_id)
            existing = {step["step_key"] for step in snapshot["steps"]}
            needed = {
                "build_world",
                *(f"direct_scene/{episode}/{scene}" for episode, scene in keys),
            }
            if snapshot["reserved_calls"] + len(needed - existing) > snapshot["call_limit"]:
                raise ExecutionConflict("invocation_budget_exhausted")
        # Temporal history carries small scope identifiers, never source or candidate bodies.
        return {
            "status": response["status"],
            "selected_scenes": response.get("selected_scenes", []),
        }

    @activity.defn(name="creation.progress")
    async def progress(self, request: dict[str, Any]) -> None:
        await self.store.progress(
            request["command_id"],
            request["status"],
            request["stage"],
            request.get("last_error"),
            can_resume=request.get("can_resume", False),
        )

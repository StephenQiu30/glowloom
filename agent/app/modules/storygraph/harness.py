from __future__ import annotations

import asyncio
import hashlib
import json
import os
import shutil
import tempfile
import time
from pathlib import Path
from typing import Any, cast

from pydantic import BaseModel

from app.candidate_runtime.schemas import (
    EpisodeAnalysisStageInput,
    EpisodeReconciliationStageInput,
    SourceEvidenceStageInput,
    StoryboardDraftStageInput,
    StoryGraphExecutionPolicy,
    StoryGraphRepairStageInput,
    StoryGraphReviewStageInput,
    StoryGraphStageInvocation,
)
from app.modules.storygraph.bundle import BundleInvalid, BundleManifest, StoryGraphBundle
from app.modules.storygraph.candidate_schemas import (
    CandidateRepairPatch,
    EpisodeAnalysisCandidate,
    EpisodeReconciliationCandidate,
    Evidence,
    SourceEvidenceCandidate,
    StoryboardRowCandidate,
    StoryGraphReviewCandidate,
)
from app.modules.storygraph.skill_registry import stage_spec


class CodexExecutionError(RuntimeError):
    pass


class CodexBudgetExceeded(CodexExecutionError):
    pass


class CodexDeadlineExceeded(CodexExecutionError):
    pass


class CodexToolPolicyViolation(CodexExecutionError):
    pass


class CodexSchemaInvalid(CodexExecutionError):
    pass


class CodexRuntimeUnavailable(CodexExecutionError):
    pass


class InvocationPolicyInvalid(CodexExecutionError):
    pass


class SkillBundleUnavailable(CodexExecutionError):
    pass


_DISABLED_FEATURES = (
    "apps",
    "browser_use",
    "browser_use_external",
    "browser_use_full_cdp_access",
    "computer_use",
    "image_generation",
    "in_app_browser",
    "multi_agent",
    "multi_agent_v2",
    "plugins",
    "shell_tool",
    "skill_search",
    "standalone_web_search",
    "unified_exec",
    "view_image",
    "web_search_cached",
    "web_search_request",
    "workspace_dependencies",
)

_SAFE_ITEM_TYPES = {"agent_message", "error", "reasoning"}


def structured_diagnostic(stdout: bytes, stderr: bytes) -> str:
    messages: list[str] = []
    for line in stdout.decode("utf-8", errors="replace").splitlines():
        try:
            decoded: Any = json.loads(line)
        except json.JSONDecodeError:
            continue
        if not isinstance(decoded, dict):
            continue
        event = cast(dict[str, Any], decoded)
        event_type = str(event.get("type", ""))
        if event_type == "error":
            message = event.get("message")
            if isinstance(message, str) and message.strip():
                messages.append(message.strip())
        error_value = event.get("error")
        if isinstance(error_value, dict):
            error = cast(dict[str, Any], error_value)
            message = error.get("message")
            if isinstance(message, str) and message.strip():
                messages.append(message.strip())
        elif event_type.endswith(".failed") and isinstance(error_value, str):
            if error_value.strip():
                messages.append(error_value.strip())
    if messages:
        return messages[-1][:400]
    fallback = [
        line.strip()
        for line in stderr.decode("utf-8", errors="replace").splitlines()
        if line.strip() not in {"", "{", "}", "[", "]"}
    ]
    return fallback[-1][:400] if fallback else "no diagnostic output"


class StoryGraphHarness:
    def __init__(
        self,
        invocation: StoryGraphStageInvocation,
        *,
        repository_root: Path | None = None,
    ) -> None:
        self.invocation = invocation
        self.bundle = StoryGraphBundle(repository_root)
        self._validate_runtime_policy(invocation.execution_policy)
        try:
            self.bundle.verify_installed_bundle()
        except BundleInvalid:
            raise
        self._model_calls = 0
        self._deadline_at = time.monotonic() + invocation.execution_policy.max_execution_seconds
        configured = os.getenv("CODEX_BIN", "").strip()
        self._codex_bin = configured or shutil.which("codex") or "codex"
        self.model_name = "codex-cli-default"

    async def execute(self) -> BaseModel:
        spec = stage_spec(self.invocation.payload.stage)
        guidance = self.bundle.guidance(
            self.invocation.payload.stage, self.invocation.payload.stage_input
        )
        prompt = json.dumps(
            self.invocation.payload.model_dump(mode="json", exclude_none=True),
            ensure_ascii=False,
            separators=(",", ":"),
            sort_keys=True,
        )
        candidate = await self._run_codex(guidance, prompt, spec.candidate_model)
        if self.invocation.payload.stage == "extract_source_evidence":
            if not isinstance(candidate, SourceEvidenceCandidate):
                raise CodexSchemaInvalid("Codex CLI returned the wrong Source Evidence schema")
            source_input = SourceEvidenceStageInput.model_validate(
                self.invocation.payload.stage_input
            )
            return normalize_source_evidence(candidate, source_input)
        if self.invocation.payload.stage == "review_storygraph":
            if not isinstance(candidate, StoryGraphReviewCandidate):
                raise CodexSchemaInvalid("Codex CLI returned the wrong StoryGraph review schema")
            review_input = StoryGraphReviewStageInput.model_validate(
                self.invocation.payload.stage_input
            )
            candidate.validate_for(review_input)
        if self.invocation.payload.stage == "analyze_episode":
            if not isinstance(candidate, EpisodeAnalysisCandidate):
                raise CodexSchemaInvalid("Codex CLI returned the wrong Episode analysis schema")
            episode_input = EpisodeAnalysisStageInput.model_validate(
                self.invocation.payload.stage_input
            )
            candidate = normalize_episode_candidate_evidence(candidate, episode_input)
            candidate.validate_for(episode_input)
        if self.invocation.payload.stage == "reconcile_episode":
            if not isinstance(candidate, EpisodeReconciliationCandidate):
                raise CodexSchemaInvalid(
                    "Codex CLI returned the wrong Episode reconciliation schema"
                )
            reconciliation_input = EpisodeReconciliationStageInput.model_validate(
                self.invocation.payload.stage_input
            )
            candidate = normalize_episode_candidate_evidence(candidate, reconciliation_input)
            candidate.validate_for(reconciliation_input)
        if self.invocation.payload.stage == "draft_storyboard":
            if not isinstance(candidate, StoryboardRowCandidate):
                raise CodexSchemaInvalid("Codex CLI returned the wrong Storyboard Draft schema")
            storyboard_input = StoryboardDraftStageInput.model_validate(
                self.invocation.payload.stage_input
            )
            candidate.validate_for(storyboard_input)
        if self.invocation.payload.stage == "repair_candidate":
            if not isinstance(candidate, CandidateRepairPatch):
                raise CodexSchemaInvalid("Codex CLI returned the wrong candidate repair schema")
            repair_input = StoryGraphRepairStageInput.model_validate(
                self.invocation.payload.stage_input
            )
            candidate.validate_for(repair_input)
        return candidate

    def _validate_runtime_policy(self, policy: StoryGraphExecutionPolicy) -> None:
        manifest = BundleManifest()
        if policy.skill_bundle_hash != manifest.skill_bundle_hash:
            raise SkillBundleUnavailable("exact StoryGraph skill bundle runtime is unavailable")
        frozen = (
            (policy.definition_key, manifest.definition_key),
            (policy.definition_version, manifest.definition_version),
            (policy.prompt_version, manifest.prompt_version),
            (policy.skill_bundle_version, manifest.skill_bundle_version),
            (policy.output_schema_version, manifest.output_schema_version),
            (policy.model_capability, manifest.model_capability),
            (policy.codex_runtime_contract, manifest.codex_runtime_contract),
        )
        if (
            any(actual != expected for actual, expected in frozen)
            or policy.allowed_tools
            or policy.max_model_calls > manifest.max_model_calls
            or policy.max_execution_seconds > manifest.max_execution_seconds
        ):
            raise InvocationPolicyInvalid(
                "StoryGraph execution policy is outside the definition manifest"
            )

    async def _run_codex(
        self,
        guidance: str,
        prompt: str,
        output_model: type[BaseModel],
    ) -> BaseModel:
        remaining_seconds = self._deadline_at - time.monotonic()
        if remaining_seconds <= 0:
            raise CodexDeadlineExceeded("Agent execution deadline is exhausted")
        if self._model_calls >= self.invocation.execution_policy.max_model_calls:
            raise CodexBudgetExceeded("Agent model-call budget is exhausted")
        self._model_calls += 1
        with tempfile.TemporaryDirectory(prefix="lanverse-codex-") as temporary:
            root = Path(temporary)
            schema_path = root / "output-schema.json"
            response_path = root / "response.json"
            schema_path.write_text(
                json.dumps(output_model.model_json_schema(), ensure_ascii=False), encoding="utf-8"
            )
            command = [
                self._codex_bin,
                "exec",
                "--ephemeral",
                "--sandbox",
                "read-only",
                "--cd",
                str(root),
                "--skip-git-repo-check",
                "--ignore-user-config",
                "--output-schema",
                str(schema_path),
                "--output-last-message",
                str(response_path),
                "--json",
                "--color",
                "never",
            ]
            for feature in _DISABLED_FEATURES:
                command.extend(["--disable", feature])
            command.append("-")
            try:
                process = await asyncio.create_subprocess_exec(
                    *command,
                    stdin=asyncio.subprocess.PIPE,
                    stdout=asyncio.subprocess.PIPE,
                    stderr=asyncio.subprocess.PIPE,
                )
            except OSError as error:
                raise CodexRuntimeUnavailable("Codex CLI could not be started") from error
            try:
                stdout, stderr = await asyncio.wait_for(
                    process.communicate(_prompt_with_guidance(guidance, prompt).encode("utf-8")),
                    timeout=remaining_seconds,
                )
            except TimeoutError as error:
                try:
                    process.kill()
                except ProcessLookupError:
                    pass
                await process.wait()
                raise CodexDeadlineExceeded("Agent execution deadline is exhausted") from error
            unauthorized_item = unauthorized_item_type(stdout)
            if unauthorized_item is not None:
                raise CodexToolPolicyViolation(
                    f"Codex CLI attempted disallowed item type: {unauthorized_item}"
                )
            if process.returncode != 0 or not response_path.is_file():
                raise CodexRuntimeUnavailable(
                    f"Codex CLI exited {process.returncode}: "
                    f"{structured_diagnostic(stdout, stderr)}"
                )
            try:
                value: Any = json.loads(response_path.read_text(encoding="utf-8"))
                return output_model.model_validate(value)
            except (OSError, json.JSONDecodeError, ValueError) as error:
                raise CodexSchemaInvalid(
                    "Codex CLI returned an invalid structured result"
                ) from error

    async def aclose(self) -> None:
        return None


def _prompt_with_guidance(guidance: str, prompt: str) -> str:
    return (
        "You are a restricted structured-text executor. No tools are authorized or available. "
        "Use only the immutable task input, explicit project guidance, and output schema supplied "
        "by the harness. Never read files, run commands, call networks, or perform side effects."
        f"\n\n# Project guidance\n{guidance}\n\n# Frozen stage input\n{prompt}"
    )


def unauthorized_item_type(stdout: bytes) -> str | None:
    for line in stdout.decode("utf-8", errors="replace").splitlines():
        try:
            decoded: Any = json.loads(line)
        except json.JSONDecodeError:
            continue
        if not isinstance(decoded, dict):
            continue
        event = cast(dict[str, Any], decoded)
        item_value = event.get("item")
        if not isinstance(item_value, dict):
            continue
        item = cast(dict[str, Any], item_value)
        item_type = item.get("type")
        if isinstance(item_type, str) and item_type not in _SAFE_ITEM_TYPES:
            return item_type[:80]
    return None


def normalize_source_evidence(
    candidate: SourceEvidenceCandidate,
    source_input: SourceEvidenceStageInput,
) -> SourceEvidenceCandidate:
    def normalize(evidence: Evidence) -> Evidence:
        start, end = evidence.source_start, evidence.source_end
        local_start, local_end = (
            start - source_input.context_start,
            end - source_input.context_start,
        )
        absolute_matches = (
            start >= source_input.context_start
            and end <= source_input.context_end
            and source_input.normalized_text[local_start:local_end] == evidence.exact_anchor
        )
        if not absolute_matches:
            local_start, local_end = start, end
            start, end = (
                source_input.context_start + local_start,
                source_input.context_start + local_end,
            )
        if (
            local_start < 0
            or local_end > len(source_input.normalized_text)
            or local_end <= local_start
            or local_end - local_start != len(evidence.exact_anchor)
            or source_input.normalized_text[local_start:local_end] != evidence.exact_anchor
        ):
            raise CodexSchemaInvalid(
                "Codex CLI returned Source Evidence outside the immutable text slice"
            )
        return evidence.model_copy(
            update={
                "source_start": start,
                "source_end": end,
                "text_hash": hashlib.sha256(evidence.exact_anchor.encode("utf-8")).hexdigest(),
            }
        )

    observations = [
        observation.model_copy(
            update={"evidence": [normalize(value) for value in observation.evidence]}
        )
        for observation in candidate.observations
    ]
    review_issues = [
        issue.model_copy(update={"evidence": [normalize(value) for value in issue.evidence]})
        for issue in candidate.review_issues
    ]
    return candidate.model_copy(
        update={"observations": observations, "review_issues": review_issues}
    )


def normalize_episode_candidate_evidence(
    candidate: EpisodeAnalysisCandidate | EpisodeReconciliationCandidate,
    stage_input: EpisodeAnalysisStageInput | EpisodeReconciliationStageInput,
) -> EpisodeAnalysisCandidate | EpisodeReconciliationCandidate:
    evidence_fields = set(Evidence.model_fields)

    def evidence_values(value: Any) -> list[Evidence]:
        found: list[Evidence] = []

        def visit(item: Any) -> None:
            if isinstance(item, dict):
                mapping = cast(dict[str, Any], item)
                if set(mapping) == evidence_fields:
                    found.append(Evidence.model_validate(mapping))
                    return
                for child in mapping.values():
                    visit(child)
            elif isinstance(item, list):
                for child in cast(list[Any], item):
                    visit(child)

        visit(value)
        return found

    trusted: dict[tuple[int, int, str, int | None], str] = {}
    if isinstance(stage_input, EpisodeAnalysisStageInput):

        def trusted_hash(evidence: Evidence) -> str:
            local_start = evidence.source_start - stage_input.context_start
            local_end = evidence.source_end - stage_input.context_start
            if (
                local_start < 0
                or local_end > len(stage_input.context_text)
                or local_end <= local_start
                or local_end - local_start != len(evidence.exact_anchor)
                or stage_input.context_text[local_start:local_end] != evidence.exact_anchor
            ):
                raise CodexSchemaInvalid(
                    "Codex CLI returned Episode Evidence outside the immutable text slice"
                )
            return hashlib.sha256(evidence.exact_anchor.encode("utf-8")).hexdigest()

    else:
        for child in stage_input.parsed_candidates():
            for evidence in evidence_values(child.model_dump(mode="python")):
                key = (
                    evidence.source_start,
                    evidence.source_end,
                    evidence.exact_anchor,
                    evidence.episode_number,
                )
                trusted[key] = evidence.text_hash

        def trusted_hash(evidence: Evidence) -> str:
            key = (
                evidence.source_start,
                evidence.source_end,
                evidence.exact_anchor,
                evidence.episode_number,
            )
            try:
                return trusted[key]
            except KeyError as error:
                raise CodexSchemaInvalid(
                    "Codex CLI returned Episode reconciliation Evidence outside its exact children"
                ) from error

    def normalize(value: Any) -> Any:
        if isinstance(value, dict):
            mapping = cast(dict[str, Any], value)
            if set(mapping) == evidence_fields:
                evidence = Evidence.model_validate(mapping)
                return {**mapping, "text_hash": trusted_hash(evidence)}
            return {key: normalize(item) for key, item in mapping.items()}
        if isinstance(value, list):
            return [normalize(item) for item in cast(list[Any], value)]
        return value

    normalized = normalize(candidate.model_dump(mode="python"))
    if isinstance(candidate, EpisodeAnalysisCandidate):
        return EpisodeAnalysisCandidate.model_validate(normalized)
    return EpisodeReconciliationCandidate.model_validate(normalized)

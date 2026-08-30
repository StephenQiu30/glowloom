from __future__ import annotations

import json
import os
import shutil
import time
from pathlib import Path

from pydantic import BaseModel

from app.candidate_runtime.scene_analysis_schemas import (
    SceneAnalysisInvocation,
    SceneFactExtractionInput,
    ScriptSpanProposalInput,
)
from app.modules.storygraph.bundle import BundleInvalid
from app.modules.storygraph.harness import (
    CodexBudgetExceeded,
    CodexSchemaInvalid,
    InvocationPolicyInvalid,
    SkillBundleUnavailable,
    run_codex_process,
)
from app.modules.storygraph.scene_analysis_bundle import SceneAnalysisBundle
from app.modules.storygraph.scene_analysis_candidates import (
    SceneFactCandidate,
    ScriptSpanCandidate,
)
from app.modules.storygraph.scene_analysis_registry import scene_analysis_stage_spec


class SceneAnalysisHarness:
    def __init__(
        self,
        invocation: SceneAnalysisInvocation,
        *,
        repository_root: Path | None = None,
    ) -> None:
        self.invocation = invocation
        self.bundle = SceneAnalysisBundle(repository_root)
        self._validate_runtime_policy()
        try:
            self.bundle.verify_installed_bundle()
        except BundleInvalid:
            raise
        self._model_calls = 0
        self._deadline_at = time.monotonic() + invocation.budget.max_execution_seconds
        configured = os.getenv("CODEX_BIN", "").strip()
        self._codex_bin = configured or shutil.which("codex") or "codex"
        self.model_name = "codex-cli-default"

    def _validate_runtime_policy(self) -> None:
        manifest = self.bundle.manifest
        if self.invocation.stage_release.bundle_hash != manifest.skill_bundle_hash:
            raise SkillBundleUnavailable("exact Scene Analysis skill bundle is unavailable")
        if (
            self.invocation.budget.max_model_calls > manifest.max_model_calls
            or self.invocation.budget.max_execution_seconds > manifest.max_execution_seconds
            or self.invocation.budget.max_output_bytes > manifest.max_output_bytes
        ):
            raise InvocationPolicyInvalid(
                "Scene Analysis execution budget is outside the release manifest"
            )

    async def execute(self) -> BaseModel:
        stage = self.invocation.payload.variant.stage_key
        spec = scene_analysis_stage_spec(stage)
        guidance = self.bundle.guidance(stage)
        prompt = json.dumps(
            self.invocation.payload.model_dump(mode="json", exclude_none=True),
            ensure_ascii=False,
            separators=(",", ":"),
            sort_keys=True,
        )
        candidate = await self._run_codex(guidance, prompt, spec.candidate_model)
        if stage == "propose_script_spans":
            if not isinstance(candidate, ScriptSpanCandidate):
                raise CodexSchemaInvalid("Codex CLI returned the wrong ScriptSpan schema")
            source = ScriptSpanProposalInput.model_validate(self.invocation.payload.stage_input)
            if candidate.source_version_id != source.source_version_id:
                raise CodexSchemaInvalid("ScriptSpan source identity drifted")
            candidate.validate_for_text(source.normalized_text)
        else:
            if not isinstance(candidate, SceneFactCandidate):
                raise CodexSchemaInvalid("Codex CLI returned the wrong SceneFact schema")
            source = SceneFactExtractionInput.model_validate(self.invocation.payload.stage_input)
            spans = ScriptSpanCandidate.model_validate(source.span_candidate)
            if (
                candidate.source_version_id != source.source_version_id
                or candidate.span_candidate_revision_id != source.span_candidate_revision_id
                or candidate.span_candidate_revision_hash != source.span_candidate_revision_hash
            ):
                raise CodexSchemaInvalid("SceneFact source identity drifted")
            candidate.validate_for_spans(source.normalized_text, spans.spans)
        size = len(
            json.dumps(
                candidate.model_dump(mode="json"),
                ensure_ascii=False,
                separators=(",", ":"),
                sort_keys=True,
            ).encode("utf-8")
        )
        if size > self.invocation.budget.max_output_bytes:
            raise CodexBudgetExceeded("Agent output byte budget is exhausted")
        return candidate

    async def _run_codex(
        self,
        guidance: str,
        prompt: str,
        output_model: type[BaseModel],
    ) -> BaseModel:
        remaining_seconds = self._deadline_at - time.monotonic()
        if self._model_calls >= self.invocation.budget.max_model_calls:
            raise CodexBudgetExceeded("Agent model-call budget is exhausted")
        self._model_calls += 1
        return await run_codex_process(
            codex_bin=self._codex_bin,
            guidance=guidance,
            prompt=prompt,
            output_model=output_model,
            timeout_seconds=remaining_seconds,
        )

    async def aclose(self) -> None:
        return None

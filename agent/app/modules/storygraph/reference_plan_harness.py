from __future__ import annotations

import json
import os
import shutil
from pathlib import Path
from typing import cast

from app.modules.storygraph.harness import InvocationPolicyInvalid
from app.modules.storygraph.reference_plan_contract import (
    ReferencePlanCandidate,
    ReferencePlanInput,
)
from app.modules.storygraph.scene_analysis_bundle import SceneAnalysisBundle
from app.reasoning.codex import CodexSchemaInvalid, run_codex_process
from app.skills.catalog import SkillCatalog


class ReferencePlanHarness:
    """Execute the single-call Reference Plan candidate boundary."""

    def __init__(
        self,
        stage_input: ReferencePlanInput,
        *,
        max_execution_seconds: int | None = None,
        max_output_bytes: int | None = None,
        repository_root: Path | None = None,
        skill_catalog: SkillCatalog | None = None,
    ) -> None:
        self.stage_input = stage_input
        catalog = skill_catalog or SkillCatalog(repository_root)
        self.bundle = cast(SceneAnalysisBundle, catalog.load("scene_analysis"))
        self.bundle.verify_installed_bundle()
        self._max_execution_seconds = (
            max_execution_seconds
            if max_execution_seconds is not None
            else self.bundle.manifest.max_execution_seconds
        )
        self._max_output_bytes = (
            max_output_bytes
            if max_output_bytes is not None
            else self.bundle.manifest.max_output_bytes
        )
        if (
            self._max_execution_seconds < 1
            or self._max_execution_seconds > self.bundle.manifest.max_execution_seconds
            or self._max_output_bytes < 1
            or self._max_output_bytes > self.bundle.manifest.max_output_bytes
        ):
            raise InvocationPolicyInvalid("Reference Plan execution budget is outside the release")
        configured = os.getenv("CODEX_BIN", "").strip()
        self._codex_bin = configured or shutil.which("codex") or "codex"
        self.model_name = "codex-cli-default"

    async def execute(self) -> ReferencePlanCandidate:
        prompt = json.dumps(
            self.stage_input.model_dump(mode="json"),
            ensure_ascii=False,
            separators=(",", ":"),
            sort_keys=True,
        )
        candidate = await run_codex_process(
            codex_bin=self._codex_bin,
            guidance=self.bundle.guidance("plan_reference_assets", "default"),
            prompt=prompt,
            output_model=ReferencePlanCandidate,
            timeout_seconds=self._max_execution_seconds,
            max_output_bytes=self._max_output_bytes,
            strict_output_schema=True,
        )
        if not isinstance(candidate, ReferencePlanCandidate):
            raise CodexSchemaInvalid("Codex CLI returned the wrong Reference Plan schema")
        try:
            candidate.validate_for(self.stage_input)
        except ValueError as error:
            raise CodexSchemaInvalid("Reference Plan Candidate changed frozen input") from error
        return candidate

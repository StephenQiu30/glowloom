from __future__ import annotations

import json
import os
import shutil
from dataclasses import dataclass
from pathlib import Path
from typing import cast

from app.modules.storygraph.harness import InvocationPolicyInvalid
from app.modules.storygraph.vision_review_bundle import VisionReviewBundle
from app.modules.storygraph.vision_review_contract import (
    VisionReviewCandidate,
)
from app.modules.storygraph.vision_review_input import VisionReviewInput
from app.reasoning.codex import (
    CodexImageInput,
    CodexMediaInvalid,
    CodexSchemaInvalid,
    run_codex_process,
)
from app.skills.catalog import SkillCatalog


@dataclass(frozen=True)
class VisionReviewMediaBinding:
    slot_key: str
    path: Path
    byte_length: int


class VisionReviewHarness:
    """Execute the single-call Vision Review candidate boundary."""

    def __init__(
        self,
        stage_input: VisionReviewInput,
        *,
        media_bindings: tuple[VisionReviewMediaBinding, ...],
        max_execution_seconds: int | None = None,
        max_output_bytes: int | None = None,
        repository_root: Path | None = None,
        skill_catalog: SkillCatalog | None = None,
    ) -> None:
        self.stage_input = VisionReviewInput.model_validate_json(stage_input.model_dump_json())
        catalog = skill_catalog or SkillCatalog(repository_root)
        self.bundle = cast(VisionReviewBundle, catalog.load("vision_review"))
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
            raise InvocationPolicyInvalid("Vision Review execution budget is outside the release")
        self.image_inputs = self._bind_images(media_bindings)
        configured = os.getenv("CODEX_BIN", "").strip()
        self._codex_bin = configured or shutil.which("codex") or "codex"
        self.model_name = "codex-cli-default"

    def _bind_images(
        self, bindings: tuple[VisionReviewMediaBinding, ...]
    ) -> tuple[CodexImageInput, ...]:
        attachments = self.stage_input.attachments
        if [value.slot_key for value in bindings] != [value.slot.slot_key for value in attachments]:
            raise CodexMediaInvalid("Vision Review media bindings do not match the complete group")
        images: list[CodexImageInput] = []
        for binding, attachment in zip(bindings, attachments, strict=True):
            if binding.byte_length != attachment.byte_length or not binding.path.is_absolute():
                raise CodexMediaInvalid("Vision Review media binding identity drifted")
            images.append(
                CodexImageInput(
                    sort_key=binding.slot_key,
                    path=binding.path,
                    content_hash=attachment.slot.sha256,
                    byte_length=attachment.byte_length,
                    media_type=attachment.media_type,
                )
            )
        return tuple(images)

    async def execute(self) -> VisionReviewCandidate:
        prompt = json.dumps(
            self.stage_input.model_dump(mode="json"),
            ensure_ascii=False,
            separators=(",", ":"),
            sort_keys=True,
        )
        candidate = await run_codex_process(
            codex_bin=self._codex_bin,
            guidance=self.bundle.guidance("review_reference_artifact", "default"),
            prompt=prompt,
            output_model=VisionReviewCandidate,
            timeout_seconds=self._max_execution_seconds,
            max_output_bytes=self._max_output_bytes,
            strict_output_schema=True,
            image_inputs=self.image_inputs,
        )
        if not isinstance(candidate, VisionReviewCandidate):
            raise CodexSchemaInvalid("Codex CLI returned the wrong Vision Review schema")
        try:
            candidate.validate_for(self.stage_input.subject)
        except ValueError as error:
            raise CodexSchemaInvalid("Vision Review Candidate changed frozen input") from error
        return candidate

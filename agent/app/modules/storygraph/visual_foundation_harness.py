from __future__ import annotations

import json
import os
import shutil
from dataclasses import dataclass
from pathlib import Path
from typing import cast
from uuid import UUID

from app.modules.storygraph.visual_foundation_bundle import VisualFoundationBundle
from app.modules.storygraph.visual_foundation_contract import (
    VisualFoundationCandidate,
    VisualFoundationInput,
)
from app.reasoning.codex import (
    CodexImageInput,
    CodexMediaInvalid,
    CodexSchemaInvalid,
    run_codex_process,
)
from app.skills.catalog import SkillCatalog


@dataclass(frozen=True)
class VisualFoundationMediaBinding:
    attachment_id: UUID
    path: Path
    byte_length: int


class VisualFoundationHarness:
    def __init__(
        self,
        stage_input: VisualFoundationInput,
        *,
        media_bindings: tuple[VisualFoundationMediaBinding, ...],
        repository_root: Path | None = None,
        skill_catalog: SkillCatalog | None = None,
    ) -> None:
        self.stage_input = stage_input
        catalog = skill_catalog or SkillCatalog(repository_root)
        self.bundle = cast(VisualFoundationBundle, catalog.load("visual_foundation"))
        self.bundle.verify_installed_bundle()
        self.image_inputs = self._bind_images(media_bindings)
        configured = os.getenv("CODEX_BIN", "").strip()
        self._codex_bin = configured or shutil.which("codex") or "codex"

    def _bind_images(
        self,
        media_bindings: tuple[VisualFoundationMediaBinding, ...],
    ) -> tuple[CodexImageInput, ...]:
        attachments = self.stage_input.reference_attachments
        supplied_ids = [value.attachment_id for value in media_bindings]
        expected_ids = [value.attachment_id for value in attachments]
        manifest = self.bundle.manifest
        if supplied_ids != expected_ids or len(supplied_ids) != len(set(supplied_ids)):
            raise CodexMediaInvalid("Visual Foundation media bindings do not match the input")
        if len(media_bindings) > manifest.max_image_inputs:
            raise CodexMediaInvalid("Visual Foundation image count exceeds its execution budget")
        if any(
            value.byte_length < 1 or value.byte_length > manifest.max_image_bytes
            for value in media_bindings
        ):
            raise CodexMediaInvalid("Visual Foundation image exceeds its byte budget")
        if sum(value.byte_length for value in media_bindings) > manifest.max_total_image_bytes:
            raise CodexMediaInvalid("Visual Foundation images exceed their aggregate byte budget")
        images: list[CodexImageInput] = []
        for attachment, binding in zip(attachments, media_bindings, strict=True):
            images.append(
                CodexImageInput(
                    sort_key=attachment.object_key,
                    path=binding.path,
                    content_hash=attachment.content_hash,
                    byte_length=binding.byte_length,
                    media_type=attachment.media_type,
                )
            )
        return tuple(images)

    async def execute(self) -> VisualFoundationCandidate:
        prompt = json.dumps(
            self.stage_input.model_dump(mode="json"),
            ensure_ascii=False,
            separators=(",", ":"),
            sort_keys=True,
        )
        candidate = await run_codex_process(
            codex_bin=self._codex_bin,
            guidance=self.bundle.guidance(),
            prompt=prompt,
            output_model=VisualFoundationCandidate,
            timeout_seconds=self.bundle.manifest.max_execution_seconds,
            max_output_bytes=self.bundle.manifest.max_output_bytes,
            strict_output_schema=True,
            image_inputs=self.image_inputs,
        )
        if not isinstance(candidate, VisualFoundationCandidate):
            raise CodexSchemaInvalid("Codex CLI returned the wrong Visual Foundation schema")
        try:
            candidate.validate_for(self.stage_input)
        except ValueError as error:
            raise CodexSchemaInvalid("Visual Foundation Candidate changed frozen input") from error
        return candidate

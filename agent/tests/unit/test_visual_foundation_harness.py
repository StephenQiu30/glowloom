from __future__ import annotations

import hashlib
from pathlib import Path
from typing import Any, cast
from uuid import UUID

import pytest

from app.modules.storygraph.visual_foundation_bundle import VisualFoundationBundle
from app.modules.storygraph.visual_foundation_contract import (
    VisualFoundationCandidate,
    VisualFoundationInput,
)
from app.modules.storygraph.visual_foundation_harness import (
    VisualFoundationHarness,
    VisualFoundationMediaBinding,
)
from app.protocol.canonical import production_canonical_hash
from app.reasoning.codex import CodexImageInput, CodexMediaInvalid
from app.skills.catalog import SkillCatalog
from tests.contract.test_visual_foundation_contract import valid_candidate, valid_input

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]


def visual_input(content: bytes) -> VisualFoundationInput:
    payload = valid_input()
    attachments = cast(list[dict[str, object]], payload["reference_attachments"])
    attachment = attachments[0]
    attachment["content_hash"] = hashlib.sha256(content).hexdigest()
    payload["reference_attachments_hash"] = production_canonical_hash(attachments)
    return VisualFoundationInput.model_validate(payload)


def visual_input_with_attachment_count(content: bytes, count: int) -> VisualFoundationInput:
    payload = valid_input()
    source = cast(list[dict[str, object]], payload["reference_attachments"])[0]
    attachments = [
        {
            **source,
            "attachment_id": str(UUID(int=48 + index)),
            "object_key": f"visual-references/project-1/reference-{index:02d}.webp",
            "content_hash": hashlib.sha256(content).hexdigest(),
        }
        for index in range(count)
    ]
    payload["reference_attachments"] = attachments
    payload["reference_attachments_hash"] = production_canonical_hash(attachments)
    return VisualFoundationInput.model_validate(payload)


def test_visual_foundation_capability_reuses_the_single_storygraph_skill() -> None:
    bundle = VisualFoundationBundle(REPOSITORY_ROOT)
    assert bundle.manifest.model_capability == "vision"
    assert bundle.manifest.max_image_inputs == 8
    assert bundle.manifest.max_image_bytes == 10 * 1024 * 1024
    assert bundle.manifest.max_total_image_bytes == 32 * 1024 * 1024
    assert bundle.loaded_paths() == ("SKILL.md", "references/visual-identity.md")
    assert bundle.verify_installed_bundle() == bundle.manifest.skill_bundle_hash

    registration = SkillCatalog(REPOSITORY_ROOT).registration("visual_foundation")
    assert registration.bundle == "build-storygraph"
    assert registration.expected_hash == bundle.manifest.skill_bundle_hash


async def test_visual_foundation_harness_binds_verified_images_and_candidate(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    content = b"RIFF" + (16).to_bytes(4, "little") + b"WEBP" + b"visual-foundation"
    image = tmp_path / "reference.webp"
    image.write_bytes(content)
    stage_input = visual_input(content)
    captured: dict[str, Any] = {}

    async def run(**kwargs: Any) -> VisualFoundationCandidate:
        captured.update(kwargs)
        return VisualFoundationCandidate.model_validate(valid_candidate(stage_input))

    monkeypatch.setattr(
        "app.modules.storygraph.visual_foundation_harness.run_codex_process",
        run,
    )
    harness = VisualFoundationHarness(
        stage_input,
        media_bindings=(
            VisualFoundationMediaBinding(
                attachment_id=UUID("00000000-0000-0000-0000-000000000030"),
                path=image,
                byte_length=len(content),
            ),
        ),
        repository_root=REPOSITORY_ROOT,
    )

    candidate = await harness.execute()

    candidate.validate_for(stage_input)
    assert captured["output_model"] is VisualFoundationCandidate
    assert captured["strict_output_schema"] is True
    assert captured["max_output_bytes"] == harness.bundle.manifest.max_output_bytes
    assert "# Visual foundation" in captured["guidance"]
    assert captured["image_inputs"] == (
        CodexImageInput(
            sort_key="visual-references/project-1/courtyard.webp",
            path=image,
            content_hash=hashlib.sha256(content).hexdigest(),
            byte_length=len(content),
            media_type="image/webp",
        ),
    )


@pytest.mark.parametrize(
    "binding_id",
    [
        None,
        UUID("00000000-0000-0000-0000-000000000099"),
    ],
)
def test_visual_foundation_harness_rejects_incomplete_or_foreign_media_bindings(
    tmp_path: Path,
    binding_id: UUID | None,
) -> None:
    content = b"RIFF" + (16).to_bytes(4, "little") + b"WEBP" + b"visual-foundation"
    image = tmp_path / "reference.webp"
    image.write_bytes(content)
    bindings = ()
    if binding_id is not None:
        bindings = (
            VisualFoundationMediaBinding(
                attachment_id=binding_id,
                path=image,
                byte_length=len(content),
            ),
        )

    with pytest.raises(CodexMediaInvalid):
        VisualFoundationHarness(
            visual_input(content),
            media_bindings=bindings,
            repository_root=REPOSITORY_ROOT,
        )


@pytest.mark.parametrize("byte_length", [0, 10 * 1024 * 1024 + 1])
def test_visual_foundation_harness_rejects_an_image_outside_its_byte_budget(
    tmp_path: Path,
    byte_length: int,
) -> None:
    content = b"RIFF" + (16).to_bytes(4, "little") + b"WEBP" + b"visual-foundation"
    image = tmp_path / "reference.webp"
    image.write_bytes(content)

    with pytest.raises(CodexMediaInvalid):
        VisualFoundationHarness(
            visual_input(content),
            media_bindings=(
                VisualFoundationMediaBinding(
                    attachment_id=UUID("00000000-0000-0000-0000-000000000030"),
                    path=image,
                    byte_length=byte_length,
                ),
            ),
            repository_root=REPOSITORY_ROOT,
        )


@pytest.mark.parametrize(
    ("count", "byte_length"),
    [
        (9, 24),
        (4, 9 * 1024 * 1024),
    ],
)
def test_visual_foundation_harness_rejects_media_outside_aggregate_budget(
    tmp_path: Path,
    count: int,
    byte_length: int,
) -> None:
    content = b"RIFF" + (16).to_bytes(4, "little") + b"WEBP" + b"visual-foundation"
    image = tmp_path / "reference.webp"
    image.write_bytes(content)
    stage_input = visual_input_with_attachment_count(content, count)
    bindings = tuple(
        VisualFoundationMediaBinding(
            attachment_id=attachment.attachment_id,
            path=image,
            byte_length=byte_length,
        )
        for attachment in stage_input.reference_attachments
    )

    with pytest.raises(CodexMediaInvalid):
        VisualFoundationHarness(
            stage_input,
            media_bindings=bindings,
            repository_root=REPOSITORY_ROOT,
        )

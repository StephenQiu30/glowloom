from __future__ import annotations

import json
from copy import deepcopy
from pathlib import Path
from typing import Any
from uuid import UUID

import pytest
from pydantic import ValidationError

from app.harness.scene_analysis_schemas import (
    SceneAnalysisControlProof,
    SceneAnalysisExecutionBudget,
    SceneAnalysisReleaseIdentity,
)
from app.harness.visual_foundation_schemas import (
    VisualFoundationInvocation,
    VisualFoundationMediaAttachment,
    VisualFoundationPayload,
    VisualFoundationScope,
    VisualFoundationShard,
    VisualFoundationStageVariant,
)
from app.modules.storygraph.bundle import SKILL_BUNDLE_HASH
from app.modules.storygraph.visual_foundation_contract import VisualFoundationInput
from tests.contract.test_visual_foundation_contract import digest, valid_input

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
INVOCATION_FIXTURE = REPOSITORY_ROOT.joinpath(
    "backend",
    "tests",
    "fixtures",
    "agent",
    "storygraph-visual-foundation-invocation.json",
)


def valid_invocation() -> VisualFoundationInvocation:
    stage_input = VisualFoundationInput.model_validate(valid_input())
    return VisualFoundationInvocation.build(
        invocation_id=UUID("00000000-0000-0000-0000-000000000080"),
        attempt_id=UUID("00000000-0000-0000-0000-000000000090"),
        stage_release=SceneAnalysisReleaseIdentity(
            skill_release_id=UUID("66666666-6666-4666-8666-666666666666"),
            skill_release_hash="1" * 64,
            stage_release_hash="2" * 64,
            bundle_content_hash=SKILL_BUNDLE_HASH,
            agent_image_digest="sha256:" + "4" * 64,
        ),
        control=SceneAnalysisControlProof(
            control_record_id=UUID("77777777-7777-4777-8777-777777777777"),
            control_revision=1,
            status="approved",
            control_hash="5" * 64,
            release_fence=0,
        ),
        budget=SceneAnalysisExecutionBudget(
            max_attempts=3,
            max_model_calls=1,
            max_execution_seconds=120,
            max_output_bytes=131072,
        ),
        payload=VisualFoundationPayload(
            variant=VisualFoundationStageVariant(
                stage_key="resolve_visual_foundation",
                profile_key="default",
                lane_key="primary",
                output_schema_version="visual-foundation-candidate-production",
            ),
            scope=VisualFoundationScope(
                workspace_id=stage_input.workspace_id,
                project_id=stage_input.project_id,
            ),
            shard=VisualFoundationShard(
                manifest_id=UUID("00000000-0000-0000-0000-000000000040"),
                manifest_hash=digest("visual-shard"),
                shard_key=f"project:{stage_input.project_id}",
                impact_closure_hash=digest("visual-impact"),
            ),
            media_attachments=[
                VisualFoundationMediaAttachment(
                    attachment_id=stage_input.reference_attachments[0].attachment_id,
                    media_object_id=UUID("00000000-0000-0000-0000-000000000031"),
                    version_no=1,
                    purpose="style_reference",
                    object_key=stage_input.reference_attachments[0].object_key,
                    content_hash=stage_input.reference_attachments[0].content_hash,
                    media_type=stage_input.reference_attachments[0].media_type,
                    byte_length=1024,
                    pixel_width=1024,
                    pixel_height=1024,
                    page_count=1,
                    frame_count=1,
                    rights_basis=stage_input.reference_attachments[0].rights_basis,
                    rights_ref_hash=stage_input.reference_attachments[0].rights_ref_hash,
                    lineage_ref_hash=digest("visual-lineage"),
                )
            ],
            stage_input=stage_input,
        ),
    )


def test_visual_foundation_invocation_freezes_project_media_and_input_hash() -> None:
    invocation = valid_invocation()
    fixture = json.loads(INVOCATION_FIXTURE.read_text(encoding="utf-8"))

    assert invocation.model_dump(mode="json") == fixture
    assert VisualFoundationInvocation.model_validate(fixture) == invocation
    assert invocation.input_hash == invocation.compute_input_hash()
    assert (
        invocation.input_hash == "c61f2dc1a56173a34ff17b7042f25ad4f3fa2ef198a520fb250e0a244ca8609c"
    )
    assert invocation.stage_instance_key() == (
        "cd6f973a527f5dd7c13a4fb759cd7edc591a37bec1778e7ce740d942eee4e8d5"
    )
    assert invocation.payload.media_attachments[0].attachment_id == (
        invocation.payload.stage_input.reference_attachments[0].attachment_id
    )


@pytest.mark.parametrize(
    ("path", "value"),
    [
        (("unexpected",), True),
        (("payload", "scope", "project_id"), "00000000-0000-0000-0000-000000000099"),
        (("payload", "shard", "shard_key"), "project:00000000-0000-0000-0000-000000000099"),
        (("payload", "media_attachments", 0, "content_hash"), "9" * 64),
        (("payload", "media_attachments", 0, "byte_length"), 10 * 1024 * 1024 + 1),
    ],
)
def test_visual_foundation_invocation_rejects_unknown_or_drifting_input(
    path: tuple[str | int, ...],
    value: object,
) -> None:
    payload: Any = deepcopy(valid_invocation().model_dump(mode="json"))
    target = payload
    for key in path[:-1]:
        target = target[key]
    target[path[-1]] = value

    with pytest.raises(ValidationError):
        VisualFoundationInvocation.model_validate(payload)

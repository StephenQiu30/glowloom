from __future__ import annotations

from dataclasses import dataclass
from typing import Literal

from pydantic import BaseModel

from app.modules.storygraph.v2_candidate_schemas import (
    SceneFactCandidateV2,
    ScriptSpanCandidateV2,
)

V2CandidateType = Literal["script_span_candidate_v2", "scene_fact_candidate_v2"]


@dataclass(frozen=True)
class StageSpecV2:
    candidate_type: V2CandidateType
    candidate_model: type[BaseModel]
    references: tuple[str, ...]


V2_REGISTRY: dict[str, StageSpecV2] = {
    "propose_script_spans": StageSpecV2(
        candidate_type="script_span_candidate_v2",
        candidate_model=ScriptSpanCandidateV2,
        references=("script-spans.md",),
    ),
    "extract_scene_facts": StageSpecV2(
        candidate_type="scene_fact_candidate_v2",
        candidate_model=SceneFactCandidateV2,
        references=("scene-facts.md",),
    ),
}


def stage_spec_v2(stage: str) -> StageSpecV2:
    try:
        return V2_REGISTRY[stage]
    except KeyError as error:
        raise ValueError("unknown StoryGraph v2 stage") from error

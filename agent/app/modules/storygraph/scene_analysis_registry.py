from __future__ import annotations

from dataclasses import dataclass
from typing import Literal

from pydantic import BaseModel

from app.modules.storygraph.scene_analysis_candidates import (
    SceneFactCandidate,
    ScriptSpanCandidate,
)

SceneAnalysisCandidateType = Literal["script_span_candidate", "scene_fact_candidate"]


@dataclass(frozen=True)
class SceneAnalysisStageSpec:
    candidate_type: SceneAnalysisCandidateType
    candidate_model: type[BaseModel]
    references: tuple[str, ...]


SCENE_ANALYSIS_REGISTRY: dict[str, SceneAnalysisStageSpec] = {
    "propose_script_spans": SceneAnalysisStageSpec(
        candidate_type="script_span_candidate",
        candidate_model=ScriptSpanCandidate,
        references=("script-spans.md",),
    ),
    "extract_scene_facts": SceneAnalysisStageSpec(
        candidate_type="scene_fact_candidate",
        candidate_model=SceneFactCandidate,
        references=("scene-facts.md",),
    ),
}


def scene_analysis_stage_spec(stage: str) -> SceneAnalysisStageSpec:
    try:
        return SCENE_ANALYSIS_REGISTRY[stage]
    except KeyError as error:
        raise ValueError("unknown Scene Analysis stage") from error

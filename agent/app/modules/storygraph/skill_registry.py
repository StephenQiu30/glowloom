from __future__ import annotations

from dataclasses import dataclass

from pydantic import BaseModel

from app.modules.storygraph.candidate_schemas import (
    CandidateRepairPatch,
    EpisodeAnalysisCandidate,
    EpisodeReconciliationCandidate,
    EpisodeSegmentationCandidate,
    ShotDetailCandidate,
    SourceEvidenceCandidate,
    StoryAnalysisCandidate,
    StoryboardRowCandidate,
    StoryGraphReviewCandidate,
    StoryReconciliationCandidate,
)


class RegistryError(ValueError):
    pass


@dataclass(frozen=True)
class StageSpec:
    candidate_type: str
    candidate_model: type[BaseModel]
    references: tuple[str, ...]


REGISTRY: dict[str, StageSpec] = {
    "extract_source_evidence": StageSpec(
        "source_evidence_candidate", SourceEvidenceCandidate, ("source-evidence.md",)
    ),
    "analyze_story": StageSpec(
        "story_analysis_candidate",
        StoryAnalysisCandidate,
        ("story-analysis.md", "entity-reconciliation.md"),
    ),
    "reconcile_story": StageSpec(
        "story_reconciliation_candidate",
        StoryReconciliationCandidate,
        ("entity-reconciliation.md", "story-analysis.md"),
    ),
    "segment_episodes": StageSpec(
        "episode_segmentation_candidate", EpisodeSegmentationCandidate, ("episode-segmentation.md",)
    ),
    "analyze_episode": StageSpec(
        "episode_analysis_candidate",
        EpisodeAnalysisCandidate,
        ("scene-structure.md", "visual-identity.md"),
    ),
    "reconcile_episode": StageSpec(
        "episode_reconciliation_candidate",
        EpisodeReconciliationCandidate,
        ("scene-structure.md", "continuity-review.md"),
    ),
    "draft_storyboard": StageSpec(
        "storyboard_row_candidate",
        StoryboardRowCandidate,
        ("storyboard-table.md", "visual-identity.md"),
    ),
    "detail_shots": StageSpec(
        "shot_detail_candidate", ShotDetailCandidate, ("shot-detail.md", "visual-identity.md")
    ),
    "review_storygraph": StageSpec(
        "storygraph_review_candidate", StoryGraphReviewCandidate, ("continuity-review.md",)
    ),
    "repair_candidate": StageSpec(
        "candidate_repair_patch", CandidateRepairPatch, ("continuity-review.md",)
    ),
}


def stage_spec(stage: str) -> StageSpec:
    try:
        return REGISTRY[stage]
    except KeyError as error:
        raise RegistryError("unknown StoryGraph stage") from error

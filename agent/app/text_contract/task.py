from __future__ import annotations

from typing import Any, Literal

from pydantic import Field

from app.text_contract.schemas import EpisodeAnalysis, EpisodeMap, Issue, Record, WorldBook
from app.text_contract.source import Digest, ResolvedEvidence, SourceEdition

Stage = Literal["map_manuscript", "analyze_episode", "build_world", "direct_scene"]


class TextTask(Record):
    invocation_id: str = Field(min_length=1, max_length=128)
    stage: Stage
    source: SourceEdition
    release_hash: Digest
    episode_map: EpisodeMap | None = None
    analyses: list[EpisodeAnalysis] = Field(default_factory=list[EpisodeAnalysis], max_length=200)
    world: WorldBook | None = None
    episode_key: str | None = Field(default=None, max_length=64)
    scene_key: str | None = Field(default=None, max_length=64)
    timeout_seconds: int = Field(default=300, ge=1, le=900)


class ContextManifest(Record):
    source_revision_id: str
    source_hash: Digest
    block_indices: list[int]
    upstream_hashes: list[Digest]
    prompt_hash: Digest
    prompt_bytes: int
    omitted: list[str]


class TextResult(Record):
    invocation_id: str
    stage: Stage
    input_hash: Digest
    release_hash: Digest
    candidate_hash: Digest
    candidate: dict[str, Any]
    evidence: list[ResolvedEvidence]
    issues: list[Issue]
    context: ContextManifest
    status: Literal["needs_review"] = "needs_review"
    usage_status: Literal["unknown"] = "unknown"
    model_calls: Literal[1] = 1

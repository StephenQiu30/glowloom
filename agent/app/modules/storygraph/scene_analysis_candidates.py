from __future__ import annotations

import hashlib
from typing import Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, model_validator


class StrictSceneAnalysisModel(BaseModel):
    model_config = ConfigDict(extra="forbid")


class SourceEvidenceSpan(StrictSceneAnalysisModel):
    source_start: int = Field(ge=0)
    source_end: int = Field(gt=0)
    text_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    exact_anchor: str = Field(min_length=1)

    @model_validator(mode="after")
    def validate_range(self) -> SourceEvidenceSpan:
        if self.source_end <= self.source_start:
            raise ValueError("evidence range must be increasing")
        return self

    def validate_for_text(self, text: str) -> None:
        if self.source_end > len(text):
            raise ValueError("evidence range exceeds source")
        anchor = text[self.source_start : self.source_end]
        if (
            anchor != self.exact_anchor
            or hashlib.sha256(anchor.encode("utf-8")).hexdigest() != self.text_hash
        ):
            raise ValueError("evidence does not match source text")


class CandidateReviewIssue(StrictSceneAnalysisModel):
    issue_key: str = Field(pattern=r"^issue_[a-z0-9_]{1,80}$")
    code: str = Field(pattern=r"^[a-z][a-z0-9_]{1,80}$")
    severity: Literal["warning", "blocking"]
    scope: str = Field(min_length=1)
    summary: str = Field(min_length=1)
    evidence: list[SourceEvidenceSpan]


class ScriptSceneSpan(StrictSceneAnalysisModel):
    temporary_span_id: str = Field(pattern=r"^span_[a-z0-9_]{1,80}$")
    kind: Literal["scene"]
    codepoint_start: int = Field(ge=0)
    codepoint_end: int = Field(gt=0)
    heading: str = Field(min_length=1)
    evidence: SourceEvidenceSpan

    @model_validator(mode="after")
    def validate_range(self) -> ScriptSceneSpan:
        if self.codepoint_end <= self.codepoint_start:
            raise ValueError("script span range must be increasing")
        if (
            self.evidence.source_start < self.codepoint_start
            or self.evidence.source_end > self.codepoint_end
        ):
            raise ValueError("script span heading evidence is outside the span")
        return self


class ScriptSpanCandidate(StrictSceneAnalysisModel):
    source_version_id: UUID
    source_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    codepoint_count: int = Field(gt=0)
    spans: list[ScriptSceneSpan] = Field(min_length=1)
    review_issues: list[CandidateReviewIssue]

    @model_validator(mode="after")
    def validate_coverage(self) -> ScriptSpanCandidate:
        previous_end = 0
        keys: set[str] = set()
        for span in self.spans:
            if span.temporary_span_id in keys or span.codepoint_start != previous_end:
                raise ValueError("script spans must be unique, ordered, and contiguous")
            keys.add(span.temporary_span_id)
            previous_end = span.codepoint_end
        if previous_end != self.codepoint_count:
            raise ValueError("script spans must cover the entire source")
        return self

    def validate_for_text(self, text: str) -> None:
        if len(text) != self.codepoint_count:
            raise ValueError("script span source length drifted")
        if hashlib.sha256(text.encode("utf-8")).hexdigest() != self.source_hash:
            raise ValueError("script span source hash drifted")
        for span in self.spans:
            span.evidence.validate_for_text(text)


class GroundedAction(StrictSceneAnalysisModel):
    text: str = Field(min_length=1)
    evidence: SourceEvidenceSpan


class GroundedDialogue(StrictSceneAnalysisModel):
    speaker_mention: str = Field(min_length=1)
    text: str = Field(min_length=1)
    evidence: SourceEvidenceSpan


class RawEntityMention(StrictSceneAnalysisModel):
    text: str = Field(min_length=1)
    evidence: SourceEvidenceSpan


class SceneFact(StrictSceneAnalysisModel):
    temporary_scene_id: str = Field(pattern=r"^scene_[a-z0-9_]{1,80}$")
    span_id: str = Field(pattern=r"^span_[a-z0-9_]{1,80}$")
    source_start: int = Field(ge=0)
    source_end: int = Field(gt=0)
    location_text: str | None
    time_text: str | None
    actions: list[GroundedAction]
    dialogues: list[GroundedDialogue]
    raw_character_mentions: list[RawEntityMention]
    raw_prop_mentions: list[RawEntityMention]

    @model_validator(mode="after")
    def validate_range(self) -> SceneFact:
        if self.source_end <= self.source_start:
            raise ValueError("scene fact range must be increasing")
        evidence = [
            *(value.evidence for value in self.actions),
            *(value.evidence for value in self.dialogues),
            *(value.evidence for value in self.raw_character_mentions),
            *(value.evidence for value in self.raw_prop_mentions),
        ]
        if any(
            value.source_start < self.source_start or value.source_end > self.source_end
            for value in evidence
        ):
            raise ValueError("scene fact evidence is outside the source span")
        return self


class SceneFactCandidate(StrictSceneAnalysisModel):
    source_version_id: UUID
    source_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    span_candidate_revision_id: UUID
    span_candidate_revision_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    scenes: list[SceneFact] = Field(min_length=1)
    review_issues: list[CandidateReviewIssue]

    @model_validator(mode="after")
    def validate_scene_keys(self) -> SceneFactCandidate:
        scene_keys = [value.temporary_scene_id for value in self.scenes]
        span_keys = [value.span_id for value in self.scenes]
        if len(scene_keys) != len(set(scene_keys)) or len(span_keys) != len(set(span_keys)):
            raise ValueError("scene facts must map one-to-one to script spans")
        return self

    def validate_for_spans(self, text: str, spans: list[ScriptSceneSpan]) -> None:
        if hashlib.sha256(text.encode("utf-8")).hexdigest() != self.source_hash:
            raise ValueError("scene fact source hash drifted")
        expected = {
            span.temporary_span_id: (span.codepoint_start, span.codepoint_end) for span in spans
        }
        supplied = {scene.span_id: (scene.source_start, scene.source_end) for scene in self.scenes}
        if supplied != expected:
            raise ValueError("scene facts do not map exactly to script spans")
        for scene in self.scenes:
            evidence = [
                *(value.evidence for value in scene.actions),
                *(value.evidence for value in scene.dialogues),
                *(value.evidence for value in scene.raw_character_mentions),
                *(value.evidence for value in scene.raw_prop_mentions),
            ]
            for value in evidence:
                value.validate_for_text(text)

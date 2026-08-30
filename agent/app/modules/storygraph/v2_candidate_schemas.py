from __future__ import annotations

import hashlib
from typing import Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, model_validator


class StrictV2Model(BaseModel):
    model_config = ConfigDict(extra="forbid")


class EvidenceSpanV2(StrictV2Model):
    source_start: int = Field(ge=0)
    source_end: int = Field(gt=0)
    text_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    exact_anchor: str = Field(min_length=1)

    @model_validator(mode="after")
    def validate_range(self) -> EvidenceSpanV2:
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


class CandidateIssueV2(StrictV2Model):
    issue_key: str = Field(pattern=r"^issue_[a-z0-9_]{1,80}$")
    code: str = Field(pattern=r"^[a-z][a-z0-9_]{1,80}$")
    severity: Literal["warning", "blocking"]
    scope: str = Field(min_length=1)
    summary: str = Field(min_length=1)
    evidence: list[EvidenceSpanV2]


class ScriptSpanV2(StrictV2Model):
    temporary_span_id: str = Field(pattern=r"^span_[a-z0-9_]{1,80}$")
    kind: Literal["scene"]
    codepoint_start: int = Field(ge=0)
    codepoint_end: int = Field(gt=0)
    heading: str = Field(min_length=1)
    evidence: EvidenceSpanV2

    @model_validator(mode="after")
    def validate_range(self) -> ScriptSpanV2:
        if self.codepoint_end <= self.codepoint_start:
            raise ValueError("script span range must be increasing")
        if (
            self.evidence.source_start < self.codepoint_start
            or self.evidence.source_end > self.codepoint_end
        ):
            raise ValueError("script span heading evidence is outside the span")
        return self


class ScriptSpanCandidateV2(StrictV2Model):
    source_version_id: UUID
    source_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    codepoint_count: int = Field(gt=0)
    spans: list[ScriptSpanV2] = Field(min_length=1)
    review_issues: list[CandidateIssueV2]

    @model_validator(mode="after")
    def validate_coverage(self) -> ScriptSpanCandidateV2:
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


class EvidenceFactV2(StrictV2Model):
    text: str = Field(min_length=1)
    evidence: EvidenceSpanV2


class DialogueFactV2(StrictV2Model):
    speaker_mention: str = Field(min_length=1)
    text: str = Field(min_length=1)
    evidence: EvidenceSpanV2


class RawMentionV2(StrictV2Model):
    text: str = Field(min_length=1)
    evidence: EvidenceSpanV2


class SceneFactV2(StrictV2Model):
    temporary_scene_id: str = Field(pattern=r"^scene_[a-z0-9_]{1,80}$")
    span_id: str = Field(pattern=r"^span_[a-z0-9_]{1,80}$")
    source_start: int = Field(ge=0)
    source_end: int = Field(gt=0)
    location_text: str | None
    time_text: str | None
    actions: list[EvidenceFactV2]
    dialogues: list[DialogueFactV2]
    raw_character_mentions: list[RawMentionV2]
    raw_prop_mentions: list[RawMentionV2]

    @model_validator(mode="after")
    def validate_range(self) -> SceneFactV2:
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


class SceneFactCandidateV2(StrictV2Model):
    source_version_id: UUID
    source_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    span_candidate_revision_id: UUID
    span_candidate_revision_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    scenes: list[SceneFactV2] = Field(min_length=1)
    review_issues: list[CandidateIssueV2]

    @model_validator(mode="after")
    def validate_scene_keys(self) -> SceneFactCandidateV2:
        scene_keys = [value.temporary_scene_id for value in self.scenes]
        span_keys = [value.span_id for value in self.scenes]
        if len(scene_keys) != len(set(scene_keys)) or len(span_keys) != len(set(span_keys)):
            raise ValueError("scene facts must map one-to-one to script spans")
        return self

    def validate_for_spans(self, text: str, spans: list[ScriptSpanV2]) -> None:
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

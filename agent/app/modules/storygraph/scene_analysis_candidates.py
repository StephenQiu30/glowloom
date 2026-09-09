from __future__ import annotations

import hashlib
from typing import Literal, cast
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, model_validator

from app.protocol.canonical import production_canonical_hash


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


class ScriptSpanCoverageProof(StrictSceneAnalysisModel):
    source_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    codepoint_start: Literal[0]
    codepoint_end: int = Field(gt=0)
    covered_codepoints: int = Field(gt=0)


class ScriptSpanCandidate(StrictSceneAnalysisModel):
    source_version_id: UUID
    source_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    codepoint_count: int = Field(gt=0)
    coverage: ScriptSpanCoverageProof
    spans: list[ScriptSceneSpan] = Field(min_length=1)
    review_issues: list[CandidateReviewIssue]

    @model_validator(mode="after")
    def validate_coverage(self) -> ScriptSpanCandidate:
        if (
            self.coverage.source_hash != self.source_hash
            or self.coverage.codepoint_end != self.codepoint_count
            or self.coverage.covered_codepoints != self.codepoint_count
        ):
            raise ValueError("script span coverage proof does not match the source")
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


class GroundedSceneAttribute(StrictSceneAnalysisModel):
    text: str = Field(min_length=1)
    evidence: SourceEvidenceSpan


class SceneFact(StrictSceneAnalysisModel):
    temporary_scene_id: str = Field(pattern=r"^scene_[a-z0-9_]{1,80}$")
    span_id: str = Field(pattern=r"^span_[a-z0-9_]{1,80}$")
    source_start: int = Field(ge=0)
    source_end: int = Field(gt=0)
    location: GroundedSceneAttribute | None
    time: GroundedSceneAttribute | None
    actions: list[GroundedAction]
    dialogues: list[GroundedDialogue]
    raw_character_mentions: list[RawEntityMention]
    raw_prop_mentions: list[RawEntityMention]

    @model_validator(mode="after")
    def validate_range(self) -> SceneFact:
        if self.source_end <= self.source_start:
            raise ValueError("scene fact range must be increasing")
        evidence = [
            *([] if self.location is None else [self.location.evidence]),
            *([] if self.time is None else [self.time.evidence]),
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
                *([] if scene.location is None else [scene.location.evidence]),
                *([] if scene.time is None else [scene.time.evidence]),
                *(value.evidence for value in scene.actions),
                *(value.evidence for value in scene.dialogues),
                *(value.evidence for value in scene.raw_character_mentions),
                *(value.evidence for value in scene.raw_prop_mentions),
            ]
            for value in evidence:
                value.validate_for_text(text)


class IdentityMentionRef(StrictSceneAnalysisModel):
    kind: Literal["character", "prop"]
    temporary_scene_id: str = Field(pattern=r"^scene_[a-z0-9_]{1,80}$")
    source_start: int = Field(ge=0)
    source_end: int = Field(gt=0)
    text_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    exact_anchor: str = Field(min_length=1)

    @model_validator(mode="after")
    def validate_range(self) -> IdentityMentionRef:
        if self.source_end <= self.source_start:
            raise ValueError("identity mention range must be increasing")
        return self


class IdentityCluster(StrictSceneAnalysisModel):
    temporary_identity_key: str = Field(pattern=r"^identity_(character|prop)_[a-z0-9_]{1,80}$")
    kind: Literal["character", "prop"]
    resolution: Literal["new", "reuse"]
    reuse_identity_key: str | None
    canonical_name: str = Field(min_length=1)
    aliases: list[str] = Field(min_length=1)
    mention_refs: list[IdentityMentionRef] = Field(min_length=1)
    supporting_evidence: list[SourceEvidenceSpan] = Field(min_length=1)
    contradicting_evidence: list[SourceEvidenceSpan]
    confidence_basis_points: int = Field(ge=0, le=10_000)
    rationale: str = Field(min_length=1)

    @model_validator(mode="after")
    def validate_identity(self) -> IdentityCluster:
        if (self.resolution == "new") != (self.reuse_identity_key is None):
            raise ValueError("identity reuse must carry exactly one allowlisted key")
        if self.reuse_identity_key is not None and not self.reuse_identity_key.strip():
            raise ValueError("identity reuse key must not be empty")
        if not self.temporary_identity_key.startswith(f"identity_{self.kind}_"):
            raise ValueError("temporary identity key does not match its kind")
        if len(self.aliases) != len(set(self.aliases)) or self.canonical_name not in self.aliases:
            raise ValueError("identity aliases must be unique and include the canonical name")
        if any(value.kind != self.kind for value in self.mention_refs):
            raise ValueError("identity cluster mixes mention kinds")
        return self


class AmbiguousIdentityMention(StrictSceneAnalysisModel):
    mention_ref: IdentityMentionRef
    candidate_identity_keys: list[str] = Field(min_length=1)
    confidence_basis_points: int = Field(ge=0, le=10_000)
    rationale: str = Field(min_length=1)

    @model_validator(mode="after")
    def validate_candidates(self) -> AmbiguousIdentityMention:
        if len(self.candidate_identity_keys) != len(set(self.candidate_identity_keys)):
            raise ValueError("ambiguous identity candidates must be unique")
        return self


class RejectedIdentityMention(StrictSceneAnalysisModel):
    mention_ref: IdentityMentionRef
    rationale: str = Field(min_length=1)


class IdentityResolutionCoverage(StrictSceneAnalysisModel):
    mention_count: int = Field(ge=0)
    resolved_count: int = Field(ge=0)
    ambiguous_count: int = Field(ge=0)
    rejected_count: int = Field(ge=0)
    mention_universe_hash: str = Field(pattern=r"^[0-9a-f]{64}$")


class IdentityResolutionCandidate(StrictSceneAnalysisModel):
    source_version_id: UUID
    source_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    scene_fact_candidate_revision_id: UUID
    scene_fact_candidate_revision_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    resolved_clusters: list[IdentityCluster]
    ambiguous_mentions: list[AmbiguousIdentityMention]
    rejected_mentions: list[RejectedIdentityMention]
    coverage: IdentityResolutionCoverage
    review_issues: list[CandidateReviewIssue]

    def validate_for_scene_facts(
        self,
        scene_facts: SceneFactCandidate,
        *,
        allowed_reuse_identity_keys: set[str],
    ) -> None:
        if (
            self.source_version_id != scene_facts.source_version_id
            or self.source_hash != scene_facts.source_hash
        ):
            raise ValueError("identity candidate source identity drifted")

        expected: dict[tuple[object, ...], IdentityMentionRef] = {}
        source_evidence: set[tuple[object, ...]] = set()
        for scene in scene_facts.scenes:
            grounded = [
                *([] if scene.location is None else [scene.location.evidence]),
                *([] if scene.time is None else [scene.time.evidence]),
                *(value.evidence for value in scene.actions),
                *(value.evidence for value in scene.dialogues),
                *(value.evidence for value in scene.raw_character_mentions),
                *(value.evidence for value in scene.raw_prop_mentions),
            ]
            source_evidence.update(_source_evidence_key(value) for value in grounded)
            for kind, mentions in (
                ("character", scene.raw_character_mentions),
                ("prop", scene.raw_prop_mentions),
            ):
                for mention in mentions:
                    ref = IdentityMentionRef(
                        kind=cast(Literal["character", "prop"], kind),
                        temporary_scene_id=scene.temporary_scene_id,
                        source_start=mention.evidence.source_start,
                        source_end=mention.evidence.source_end,
                        text_hash=mention.evidence.text_hash,
                        exact_anchor=mention.evidence.exact_anchor,
                    )
                    key = _identity_mention_key(ref)
                    if key in expected:
                        raise ValueError("scene facts contain a duplicated raw mention")
                    expected[key] = ref

        supplied: list[IdentityMentionRef] = []
        identity_kinds: dict[str, Literal["character", "prop"]] = {}
        resolved_count = 0
        for cluster in self.resolved_clusters:
            if cluster.temporary_identity_key in identity_kinds:
                raise ValueError("identity cluster key is duplicated")
            identity_kinds[cluster.temporary_identity_key] = cluster.kind
            if (
                cluster.resolution == "reuse"
                and cluster.reuse_identity_key not in allowed_reuse_identity_keys
            ):
                raise ValueError("identity reuse key is outside the input allowlist")
            if any(
                _source_evidence_key(value) not in source_evidence
                for value in (*cluster.supporting_evidence, *cluster.contradicting_evidence)
            ):
                raise ValueError("identity evidence is not present in the frozen SceneFacts")
            supplied.extend(cluster.mention_refs)
            resolved_count += len(cluster.mention_refs)
        for ambiguous in self.ambiguous_mentions:
            if any(key not in identity_kinds for key in ambiguous.candidate_identity_keys):
                raise ValueError("ambiguous mention references an unknown identity cluster")
            if any(
                identity_kinds[key] != ambiguous.mention_ref.kind
                for key in ambiguous.candidate_identity_keys
            ):
                raise ValueError("ambiguous mention references a different identity kind")
            supplied.append(ambiguous.mention_ref)
        supplied.extend(value.mention_ref for value in self.rejected_mentions)

        supplied_keys = [_identity_mention_key(value) for value in supplied]
        if len(supplied_keys) != len(set(supplied_keys)) or set(supplied_keys) != set(expected):
            raise ValueError("identity candidate does not partition every raw mention exactly once")
        for value in supplied:
            if value != expected[_identity_mention_key(value)]:
                raise ValueError("identity mention drifted from its SceneFact evidence")

        ordered_universe = [expected[key].model_dump(mode="json") for key in sorted(expected)]
        coverage = self.coverage
        if (
            coverage.mention_count != len(expected)
            or coverage.resolved_count != resolved_count
            or coverage.ambiguous_count != len(self.ambiguous_mentions)
            or coverage.rejected_count != len(self.rejected_mentions)
            or coverage.resolved_count + coverage.ambiguous_count + coverage.rejected_count
            != coverage.mention_count
            or coverage.mention_universe_hash != production_canonical_hash(ordered_universe)
        ):
            raise ValueError("identity mention coverage proof is invalid")


def _identity_mention_key(value: IdentityMentionRef) -> tuple[object, ...]:
    return (
        value.kind,
        value.temporary_scene_id,
        value.source_start,
        value.source_end,
        value.text_hash,
        value.exact_anchor,
    )


def _source_evidence_key(value: SourceEvidenceSpan) -> tuple[object, ...]:
    return (value.source_start, value.source_end, value.text_hash, value.exact_anchor)

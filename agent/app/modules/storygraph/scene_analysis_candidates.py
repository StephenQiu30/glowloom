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


class StructureIdentityReviewSuggestion(StrictSceneAnalysisModel):
    issue_key: str = Field(pattern=r"^issue_[a-z0-9_]{1,80}$")
    action: Literal[
        "inspect_source",
        "adjust_episode_boundary",
        "adjust_scene_boundary",
        "separate_identity",
        "merge_identity",
        "resolve_mention",
        "reject_mention",
    ]
    target_keys: list[str] = Field(min_length=1)
    rationale: str = Field(min_length=1)

    @model_validator(mode="after")
    def validate_targets(self) -> StructureIdentityReviewSuggestion:
        if self.target_keys != sorted(set(self.target_keys)) or any(
            not value.strip() for value in self.target_keys
        ):
            raise ValueError("review suggestion targets must be sorted and unique")
        return self


class ScriptEpisodeSpan(StrictSceneAnalysisModel):
    temporary_episode_id: str = Field(pattern=r"^episode_[a-z0-9_]{1,80}$")
    position: int = Field(ge=1)
    codepoint_start: int = Field(ge=0)
    codepoint_end: int = Field(gt=0)
    heading: str | None
    evidence: SourceEvidenceSpan | None
    scene_span_ids: list[str] = Field(min_length=1)

    @model_validator(mode="after")
    def validate_episode(self) -> ScriptEpisodeSpan:
        if self.codepoint_end <= self.codepoint_start:
            raise ValueError("episode span range must be increasing")
        if (self.heading is None) != (self.evidence is None):
            raise ValueError("episode heading and evidence must be present together")
        if self.heading is not None and not self.heading.strip():
            raise ValueError("episode heading must not be empty")
        if self.evidence is not None and (
            self.evidence.source_start < self.codepoint_start
            or self.evidence.source_end > self.codepoint_end
        ):
            raise ValueError("episode heading evidence is outside the span")
        if len(self.scene_span_ids) != len(set(self.scene_span_ids)) or any(
            not value.startswith("span_") for value in self.scene_span_ids
        ):
            raise ValueError("episode scene span keys must be unique")
        return self


class ScriptSceneSpan(StrictSceneAnalysisModel):
    temporary_span_id: str = Field(pattern=r"^span_[a-z0-9_]{1,80}$")
    episode_span_id: str = Field(pattern=r"^episode_[a-z0-9_]{1,80}$")
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
    episodes: list[ScriptEpisodeSpan] = Field(min_length=1)
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
        previous_episode_end = 0
        episode_keys: set[str] = set()
        expected_scene_keys: dict[str, list[str]] = {}
        episode_bounds: dict[str, tuple[int, int]] = {}
        for index, episode in enumerate(self.episodes, start=1):
            if (
                episode.temporary_episode_id in episode_keys
                or episode.position != index
                or episode.codepoint_start != previous_episode_end
            ):
                raise ValueError("episode spans must be unique, ordered, and contiguous")
            episode_keys.add(episode.temporary_episode_id)
            expected_scene_keys[episode.temporary_episode_id] = episode.scene_span_ids
            episode_bounds[episode.temporary_episode_id] = (
                episode.codepoint_start,
                episode.codepoint_end,
            )
            previous_episode_end = episode.codepoint_end
        if previous_episode_end != self.codepoint_count:
            raise ValueError("episode spans must cover the entire source")

        previous_end = 0
        keys: set[str] = set()
        supplied_scene_keys: dict[str, list[str]] = {
            key: [] for key in expected_scene_keys
        }
        for span in self.spans:
            bounds = episode_bounds.get(span.episode_span_id)
            if (
                span.temporary_span_id in keys
                or span.codepoint_start != previous_end
                or bounds is None
                or span.codepoint_start < bounds[0]
                or span.codepoint_end > bounds[1]
            ):
                raise ValueError("script spans must be unique, ordered, and contiguous")
            keys.add(span.temporary_span_id)
            supplied_scene_keys[span.episode_span_id].append(span.temporary_span_id)
            previous_end = span.codepoint_end
        if previous_end != self.codepoint_count:
            raise ValueError("script spans must cover the entire source")
        if supplied_scene_keys != expected_scene_keys:
            raise ValueError("every scene span must belong to exactly one episode")
        return self

    def validate_for_text(self, text: str) -> None:
        if len(text) != self.codepoint_count:
            raise ValueError("script span source length drifted")
        if hashlib.sha256(text.encode("utf-8")).hexdigest() != self.source_hash:
            raise ValueError("script span source hash drifted")
        for episode in self.episodes:
            if episode.evidence is not None:
                episode.evidence.validate_for_text(text)
                if episode.heading != episode.evidence.exact_anchor:
                    raise ValueError("episode heading evidence does not match its heading")
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
    kind: Literal["character", "location", "prop"]
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
    temporary_identity_key: str = Field(
        pattern=r"^identity_(character|location|prop)_[a-z0-9_]{1,80}$"
    )
    kind: Literal["character", "location", "prop"]
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
            if scene.location is not None:
                location = scene.location.evidence
                ref = IdentityMentionRef(
                    kind="location",
                    temporary_scene_id=scene.temporary_scene_id,
                    source_start=location.source_start,
                    source_end=location.source_end,
                    text_hash=location.text_hash,
                    exact_anchor=location.exact_anchor,
                )
                key = _identity_mention_key(ref)
                if key in expected:
                    raise ValueError("scene facts contain a duplicated raw mention")
                expected[key] = ref
            for kind, mentions in (
                ("character", scene.raw_character_mentions),
                ("prop", scene.raw_prop_mentions),
            ):
                for mention in mentions:
                    ref = IdentityMentionRef(
                        kind=cast(Literal["character", "location", "prop"], kind),
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
        identity_kinds: dict[str, Literal["character", "location", "prop"]] = {}
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


class StructureIdentityReviewCandidate(StrictSceneAnalysisModel):
    profile_key: Literal["structure_identity"]
    source_version_id: UUID
    source_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    span_candidate_revision_id: UUID
    span_candidate_revision_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    scene_fact_candidate_revision_id: UUID
    scene_fact_candidate_revision_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    identity_candidate_revision_id: UUID
    identity_candidate_revision_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    review_issues: list[CandidateReviewIssue]
    suggestions: list[StructureIdentityReviewSuggestion]

    @model_validator(mode="after")
    def validate_ordering(self) -> StructureIdentityReviewCandidate:
        issue_keys = [value.issue_key for value in self.review_issues]
        if issue_keys != sorted(set(issue_keys)):
            raise ValueError("structure identity review issues must be unique and sorted")
        suggestion_keys = [value.issue_key for value in self.suggestions]
        if suggestion_keys != sorted(set(suggestion_keys)):
            raise ValueError("structure identity review suggestions must be unique and sorted")
        if any(value not in set(issue_keys) for value in suggestion_keys):
            raise ValueError("review suggestion references an unknown issue")
        return self

    def validate_for(self, value: object) -> None:
        from app.harness.scene_analysis_schemas import StructureIdentityReviewInput

        if not isinstance(value, StructureIdentityReviewInput):
            raise ValueError("structure identity review input is invalid")
        if (
            self.source_version_id != value.source_version_id
            or self.source_hash != value.source_hash
            or self.span_candidate_revision_id != value.span_candidate_revision_id
            or self.span_candidate_revision_hash != value.span_candidate_revision_hash
            or self.scene_fact_candidate_revision_id != value.scene_fact_candidate_revision_id
            or self.scene_fact_candidate_revision_hash != value.scene_fact_candidate_revision_hash
            or self.identity_candidate_revision_id != value.identity_candidate_revision_id
            or self.identity_candidate_revision_hash != value.identity_candidate_revision_hash
        ):
            raise ValueError("structure identity review target drifted")
        supplied_by_key = {issue.issue_key: issue for issue in self.review_issues}
        if any(
            supplied_by_key.get(issue.issue_key) != issue
            for issue in value.deterministic_issues
        ):
            raise ValueError("structure identity review changed a deterministic issue")
        deterministic_keys = {issue.issue_key for issue in value.deterministic_issues}
        for issue in self.review_issues:
            for evidence in issue.evidence:
                evidence.validate_for_text(value.normalized_text)
            if issue.issue_key not in deterministic_keys and not issue.evidence:
                raise ValueError("semantic review issue must carry source evidence")


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

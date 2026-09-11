from __future__ import annotations

import hashlib
import math
import re
from typing import TYPE_CHECKING, Literal, cast
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, model_validator

from app.protocol.canonical import production_canonical_hash

if TYPE_CHECKING:
    from app.harness.scene_analysis_schemas import FrozenStructureIdentityMentionMapping


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
        supplied_scene_keys: dict[str, list[str]] = {key: [] for key in expected_scene_keys}
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
    occurrence_role: Literal["actual", "mentioned_only"]
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
    occurrence_role: Literal["actual", "mentioned_only"]
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
                    occurrence_role="actual",
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
                        occurrence_role=mention.occurrence_role,
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
            supplied_by_key.get(issue.issue_key) != issue for issue in value.deterministic_issues
        ):
            raise ValueError("structure identity review changed a deterministic issue")
        deterministic_keys = {issue.issue_key for issue in value.deterministic_issues}
        for issue in self.review_issues:
            for evidence in issue.evidence:
                evidence.validate_for_text(value.normalized_text)
            if issue.issue_key not in deterministic_keys and not issue.evidence:
                raise ValueError("semantic review issue must carry source evidence")


class CreatorDecisionProposal(StrictSceneAnalysisModel):
    decision_key: str = Field(pattern=r"^decision_[a-z0-9_]{1,100}$")
    rationale: str = Field(min_length=1)


class ProductionSourceBasis(StrictSceneAnalysisModel):
    provenance: Literal["source_explicit", "inferred", "user_supplied"]
    evidence: list[SourceEvidenceSpan]
    creator_decision_proposal: CreatorDecisionProposal | None

    @model_validator(mode="after")
    def validate_exactly_one_source(self) -> ProductionSourceBasis:
        if bool(self.evidence) == (self.creator_decision_proposal is not None):
            raise ValueError("production entity source must be Evidence XOR CreatorDecision")
        if self.provenance == "user_supplied" and self.creator_decision_proposal is None:
            raise ValueError("user-supplied production entity data requires a decision proposal")
        if self.provenance != "user_supplied" and self.creator_decision_proposal is not None:
            raise ValueError("creator decisions must be marked user_supplied")
        return self


class ProductionSemanticSlot(StrictSceneAnalysisModel):
    slot_key: str = Field(pattern=r"^[a-z][a-z0-9_]{1,80}$")
    resolution: Literal["known", "unspecified_design_gap", "not_applicable"]
    value: str | None
    design_gap_key: str | None

    @model_validator(mode="after")
    def validate_resolution(self) -> ProductionSemanticSlot:
        if self.resolution == "known":
            if self.value is None or not self.value.strip() or self.design_gap_key is not None:
                raise ValueError("known production slot requires only a value")
        elif self.resolution == "unspecified_design_gap":
            if self.value is not None or self.design_gap_key is None:
                raise ValueError("unspecified production slot requires only a DesignGap")
        elif self.value is not None or self.design_gap_key is not None:
            raise ValueError("not-applicable production slot cannot carry a value or DesignGap")
        return self


class ProductionStateFragment(StrictSceneAnalysisModel):
    state_key: str = Field(pattern=r"^state_[a-z0-9_]{1,120}$")
    state_kind: Literal["character_appearance", "location_state", "prop_state"]
    complete_slots: list[ProductionSemanticSlot] = Field(min_length=1)
    applicable_scene_scope_keys: list[str] = Field(min_length=1)
    entry_reason: str = Field(min_length=1)
    exit_reason: str = Field(min_length=1)
    previous_state_key: str | None
    next_state_key: str | None
    basis: ProductionSourceBasis

    @model_validator(mode="after")
    def validate_state(self) -> ProductionStateFragment:
        slot_keys = [value.slot_key for value in self.complete_slots]
        if slot_keys != sorted(set(slot_keys)):
            raise ValueError("production state slots must be sorted and unique")
        if self.applicable_scene_scope_keys != sorted(set(self.applicable_scene_scope_keys)):
            raise ValueError("production state scopes must be sorted and unique")
        if any(not value.startswith("scene:") for value in self.applicable_scene_scope_keys):
            raise ValueError("production state scope must reference a formal Scene")
        return self


class ProductionEntityFragment(StrictSceneAnalysisModel):
    identity_key: str = Field(min_length=1)
    kind: Literal["character", "location", "prop"]
    specification_key: str = Field(pattern=r"^specification_[a-z0-9_]{1,120}$")
    specification_slots: list[ProductionSemanticSlot] = Field(min_length=1)
    basis: ProductionSourceBasis
    states: list[ProductionStateFragment] = Field(min_length=1)

    @model_validator(mode="after")
    def validate_fragment(self) -> ProductionEntityFragment:
        slot_keys = [value.slot_key for value in self.specification_slots]
        state_keys = [value.state_key for value in self.states]
        expected_state_kind = {
            "character": "character_appearance",
            "location": "location_state",
            "prop": "prop_state",
        }[self.kind]
        if slot_keys != sorted(set(slot_keys)):
            raise ValueError("production specification slots must be sorted and unique")
        if state_keys != sorted(set(state_keys)) or any(
            value.state_kind != expected_state_kind for value in self.states
        ):
            raise ValueError("production states must be typed, sorted, and unique")
        for index, state in enumerate(self.states):
            previous_key = None if index == 0 else self.states[index - 1].state_key
            next_key = None if index == len(self.states) - 1 else self.states[index + 1].state_key
            if state.previous_state_key != previous_key or state.next_state_key != next_key:
                raise ValueError("production state lineage must be a complete ordered chain")
        return self


class ProductionWorldClaimParticipant(StrictSceneAnalysisModel):
    role: Literal["subject", "object", "participant"]
    identity_key: str = Field(min_length=1)


class ProductionWorldClaimAnchor(StrictSceneAnalysisModel):
    role: Literal["episode", "scene", "beat"]
    target_key: str = Field(min_length=1)


class ProductionWorldClaimScope(StrictSceneAnalysisModel):
    kind: Literal["project", "episode", "scene", "beat"]
    owner_logical_id: str = Field(min_length=1)


class ProductionWorldStoryTimeRange(StrictSceneAnalysisModel):
    start_key: str = Field(min_length=1)
    end_key: str = Field(min_length=1)

    @model_validator(mode="after")
    def validate_ordering(self) -> ProductionWorldStoryTimeRange:
        if self.start_key > self.end_key or any(
            ord(character) <= 0x1F or ord(character) == 0x7F
            for value in (self.start_key, self.end_key)
            for character in value
        ):
            raise ValueError("production Narrative Claim story time is invalid")
        return self


class ProductionWorldNarrativeClaim(StrictSceneAnalysisModel):
    claim_series_key: str = Field(pattern=r"^claim_[a-z0-9_]{1,120}$")
    predicate: str = Field(pattern=r"^[a-z][a-z0-9_]{0,63}$")
    anchors: list[ProductionWorldClaimAnchor] = Field(min_length=1)
    valid_scope: ProductionWorldClaimScope
    story_time_range: ProductionWorldStoryTimeRange | None
    polarity: Literal["positive", "negative", "neutral"]
    status: Literal["asserted", "negated"]

    @model_validator(mode="after")
    def validate_anchors(self) -> ProductionWorldNarrativeClaim:
        keys = [(value.target_key, value.role) for value in self.anchors]
        if keys != sorted(set(keys)):
            raise ValueError("production Narrative Claim anchors must be sorted and unique")
        return self


class ProductionWorldClaimFragment(StrictSceneAnalysisModel):
    claim_key: str = Field(pattern=r"^claim_[a-z0-9_]{1,120}$")
    claim_type: Literal[
        "world_rule", "relationship", "foreshadowing", "payoff", "story_arc", "plot_thread"
    ]
    participants: list[ProductionWorldClaimParticipant] = Field(min_length=1)
    statement: str = Field(min_length=1)
    narrative: ProductionWorldNarrativeClaim | None
    basis: ProductionSourceBasis

    @model_validator(mode="after")
    def validate_claim(self) -> ProductionWorldClaimFragment:
        keys = [(value.identity_key, value.role) for value in self.participants]
        if keys != sorted(set(keys)):
            raise ValueError("production world claim participants must be sorted and unique")
        identities = [value.identity_key for value in self.participants]
        if len(identities) != len(set(identities)):
            raise ValueError("production world claim participant identities must be unique")
        roles = [value.role for value in self.participants]
        if roles.count("subject") != 1 or roles.count("object") > 1:
            raise ValueError("production world claim participant roles are invalid")
        narrative_type = self.claim_type in {"relationship", "foreshadowing", "payoff"}
        if narrative_type != (self.narrative is not None):
            raise ValueError("production Narrative Claim facts are incomplete")
        if self.narrative is not None and self.narrative.claim_series_key != self.claim_key:
            raise ValueError("production Narrative Claim series must equal its stable claim key")
        return self


class ProductionDesignGap(StrictSceneAnalysisModel):
    gap_key: str = Field(pattern=r"^gap_[a-z0-9_]{1,120}$")
    subject_key: str = Field(min_length=1)
    field_key: str = Field(pattern=r"^[a-z][a-z0-9_]{1,80}$")
    missing_reason: str = Field(min_length=1)
    source_constraints: list[SourceEvidenceSpan]
    mutually_exclusive_options: list[str]
    impacted_scene_scope_keys: list[str] = Field(min_length=1)
    allowed_resolution_sources: list[Literal["creator_decision", "visual_foundation"]] = Field(
        min_length=1
    )

    @model_validator(mode="after")
    def validate_gap(self) -> ProductionDesignGap:
        if self.mutually_exclusive_options != sorted(set(self.mutually_exclusive_options)):
            raise ValueError("DesignGap options must be sorted and unique")
        if self.impacted_scene_scope_keys != sorted(set(self.impacted_scene_scope_keys)):
            raise ValueError("DesignGap scopes must be sorted and unique")
        if self.allowed_resolution_sources != sorted(set(self.allowed_resolution_sources)):
            raise ValueError("DesignGap resolution sources must be sorted and unique")
        return self


class ProductionEntityFragmentCandidate(StrictSceneAnalysisModel):
    source_version_id: UUID
    source_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    structure_identity_set_version_id: UUID
    structure_identity_set_version_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    scene_fact_candidate_revision_id: UUID
    scene_fact_candidate_revision_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    entities: list[ProductionEntityFragment] = Field(min_length=1)
    world_claims: list[ProductionWorldClaimFragment]
    design_gaps: list[ProductionDesignGap]
    review_issues: list[CandidateReviewIssue]

    @model_validator(mode="after")
    def validate_ordering(self) -> ProductionEntityFragmentCandidate:
        entity_keys = [value.identity_key for value in self.entities]
        claim_keys = [value.claim_key for value in self.world_claims]
        gap_keys = [value.gap_key for value in self.design_gaps]
        issue_keys = [value.issue_key for value in self.review_issues]
        if entity_keys != sorted(set(entity_keys)):
            raise ValueError("production entities must be sorted and unique")
        if claim_keys != sorted(set(claim_keys)):
            raise ValueError("production world claims must be sorted and unique")
        if gap_keys != sorted(set(gap_keys)):
            raise ValueError("production DesignGaps must be sorted and unique")
        if issue_keys != sorted(set(issue_keys)):
            raise ValueError("production entity review issues must be sorted and unique")
        return self

    def validate_for(self, value: object) -> None:
        from app.harness.scene_analysis_schemas import ProductionEntityDerivationInput

        if not isinstance(value, ProductionEntityDerivationInput):
            raise ValueError("production entity derivation input is invalid")
        if (
            self.source_version_id != value.source_version_id
            or self.source_hash != value.source_hash
            or self.structure_identity_set_version_id != value.structure_identity_set_version_id
            or self.structure_identity_set_version_hash != value.structure_identity_set_version_hash
            or self.scene_fact_candidate_revision_id != value.scene_fact_candidate_revision_id
            or self.scene_fact_candidate_revision_hash != value.scene_fact_candidate_revision_hash
        ):
            raise ValueError("production entity lineage drifted")

        expected_identities = {
            item.identity_key: item.kind for item in value.structure_identity_set.identities
        }
        supplied_identities = {item.identity_key: item.kind for item in self.entities}
        if supplied_identities != expected_identities:
            raise ValueError(
                "production entity candidate must cover every formal identity exactly once"
            )
        allowed_scopes = {item.scope_key for item in value.structure_identity_set.scene_refs}
        evidence_universe = value.scene_fact_evidence_universe()
        subject_keys = set(expected_identities)
        gap_keys = {gap.gap_key for gap in self.design_gaps}

        for entity in self.entities:
            subject_keys.add(entity.specification_key)
            self._validate_basis(entity.basis, value.normalized_text, evidence_universe)
            for slot in entity.specification_slots:
                if slot.design_gap_key is not None and slot.design_gap_key not in gap_keys:
                    raise ValueError("production specification references an unknown DesignGap")
            for state in entity.states:
                subject_keys.add(state.state_key)
                if not set(state.applicable_scene_scope_keys).issubset(allowed_scopes):
                    raise ValueError("production state references an unknown formal Scene")
                self._validate_basis(state.basis, value.normalized_text, evidence_universe)
                for slot in state.complete_slots:
                    if slot.design_gap_key is not None and slot.design_gap_key not in gap_keys:
                        raise ValueError("production state references an unknown DesignGap")
        for claim in self.world_claims:
            if not {item.identity_key for item in claim.participants}.issubset(expected_identities):
                raise ValueError("production world claim references an unknown formal identity")
            if claim.narrative is not None:
                episode_ids = {
                    str(item.episode_id) for item in value.structure_identity_set.episode_refs
                }
                scene_ids = {
                    str(item.scene_owner_logical_id)
                    for item in value.structure_identity_set.scene_refs
                }
                for anchor in claim.narrative.anchors:
                    if not self._valid_claim_anchor(anchor, episode_ids, scene_ids):
                        raise ValueError(
                            "production Narrative Claim references an unknown formal anchor"
                        )
                scope = claim.narrative.valid_scope
                scope_valid = (
                    (
                        scope.kind == "project"
                        and scope.owner_logical_id == str(value.structure_identity_set.project_id)
                    )
                    or (scope.kind == "episode" and scope.owner_logical_id in episode_ids)
                    or (
                        scope.kind == "scene"
                        and scope.owner_logical_id.startswith("scene:")
                        and scope.owner_logical_id.removeprefix("scene:") in scene_ids
                    )
                    or (
                        scope.kind == "beat"
                        and any(
                            anchor.role == "beat" and anchor.target_key == scope.owner_logical_id
                            for anchor in claim.narrative.anchors
                        )
                    )
                )
                if not scope_valid:
                    raise ValueError("production Narrative Claim scope is not formal")
            self._validate_basis(claim.basis, value.normalized_text, evidence_universe)
        for gap in self.design_gaps:
            if gap.subject_key not in subject_keys or not set(
                gap.impacted_scene_scope_keys
            ).issubset(allowed_scopes):
                raise ValueError("production DesignGap references an unknown subject or Scene")
            for evidence in gap.source_constraints:
                self._validate_evidence(evidence, value.normalized_text, evidence_universe)
        for issue in self.review_issues:
            for evidence in issue.evidence:
                self._validate_evidence(evidence, value.normalized_text, evidence_universe)

    @staticmethod
    def _validate_basis(
        basis: ProductionSourceBasis,
        normalized_text: str,
        evidence_universe: set[tuple[object, ...]],
    ) -> None:
        for evidence in basis.evidence:
            ProductionEntityFragmentCandidate._validate_evidence(
                evidence, normalized_text, evidence_universe
            )

    @staticmethod
    def _valid_claim_anchor(
        value: ProductionWorldClaimAnchor,
        episode_ids: set[str],
        scene_ids: set[str],
    ) -> bool:
        if value.role == "episode":
            return value.target_key.removeprefix(
                "episode:"
            ) in episode_ids and value.target_key.startswith("episode:")
        if value.role == "scene":
            return value.target_key.removeprefix(
                "scene:"
            ) in scene_ids and value.target_key.startswith("scene:")
        match = re.fullmatch(r"beat:([0-9a-f-]{36}):(beat_[a-z0-9_]{1,120})", value.target_key)
        return match is not None and match.group(1) in scene_ids

    @staticmethod
    def _validate_evidence(
        evidence: SourceEvidenceSpan,
        normalized_text: str,
        evidence_universe: set[tuple[object, ...]],
    ) -> None:
        evidence.validate_for_text(normalized_text)
        if _source_evidence_key(evidence) not in evidence_universe:
            raise ValueError("production entity Evidence is outside the frozen SceneFacts")


def _identity_mention_key(value: IdentityMentionRef) -> tuple[object, ...]:
    return (
        value.kind,
        value.occurrence_role,
        value.temporary_scene_id,
        value.source_start,
        value.source_end,
        value.text_hash,
        value.exact_anchor,
    )


def _source_evidence_key(value: SourceEvidenceSpan) -> tuple[object, ...]:
    return (value.source_start, value.source_end, value.text_hash, value.exact_anchor)


class SceneDialogueFragment(StrictSceneAnalysisModel):
    dialogue_key: str = Field(pattern=r"^dialogue_[a-z0-9_]{1,120}$")
    order: int = Field(ge=1)
    speaker_identity_key: str | None
    speaker_evidence: SourceEvidenceSpan | None
    text: str = Field(min_length=1)
    evidence: SourceEvidenceSpan

    @model_validator(mode="after")
    def validate_speaker(self) -> SceneDialogueFragment:
        if (self.speaker_identity_key is None) != (self.speaker_evidence is None):
            raise ValueError("dialogue speaker identity and Evidence must be supplied together")
        return self


class SceneBeatFragment(StrictSceneAnalysisModel):
    beat_key: str = Field(pattern=r"^beat_[a-z0-9_]{1,120}$")
    order: int = Field(ge=1)
    text: str = Field(min_length=1)
    evidence: SourceEvidenceSpan


class SceneOccurrenceFragment(StrictSceneAnalysisModel):
    occurrence_key: str = Field(pattern=r"^occurrence_[a-z0-9_]{1,120}$")
    order: int = Field(ge=1)
    subject_kind: Literal["character", "location", "prop"]
    identity_key: str = Field(min_length=1)
    state_key: str = Field(pattern=r"^state_[a-z0-9_]{1,120}$")
    occurrence_role: Literal["actual", "mentioned_only"]
    evidence: SourceEvidenceSpan


class SceneBindingFragment(StrictSceneAnalysisModel):
    scene_scope_key: str = Field(pattern=r"^scene:[0-9a-f-]{36}$")
    scene_owner_logical_id: UUID
    temporary_scene_id: str = Field(pattern=r"^scene_[a-z0-9_]{1,80}$")
    source_start: int = Field(ge=0)
    source_end: int = Field(gt=0)
    dialogues: list[SceneDialogueFragment]
    beats: list[SceneBeatFragment]
    occurrences: list[SceneOccurrenceFragment]

    @model_validator(mode="after")
    def validate_ordering(self) -> SceneBindingFragment:
        if self.source_end <= self.source_start:
            raise ValueError("Scene binding range must be increasing")
        ordered_keys = (
            [(value.order, value.dialogue_key) for value in self.dialogues],
            [(value.order, value.beat_key) for value in self.beats],
            [(value.order, value.occurrence_key) for value in self.occurrences],
        )
        for values in ordered_keys:
            if [order for order, _ in values] != list(range(1, len(values) + 1)):
                raise ValueError("Scene binding order must be contiguous")
            keys = [key for _, key in values]
            if len(keys) != len(set(keys)):
                raise ValueError("Scene binding keys must be unique")
        if [value.evidence.source_start for value in self.occurrences] != sorted(
            value.evidence.source_start for value in self.occurrences
        ):
            raise ValueError("Scene occurrences must follow source order")
        return self


class SceneBindingFragmentCandidate(StrictSceneAnalysisModel):
    source_version_id: UUID
    source_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    structure_identity_set_version_id: UUID
    structure_identity_set_version_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    scene_fact_candidate_revision_id: UUID
    scene_fact_candidate_revision_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    production_entity_candidate_revision_id: UUID
    production_entity_candidate_revision_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    scenes: list[SceneBindingFragment] = Field(min_length=1)
    review_issues: list[CandidateReviewIssue]

    def validate_for_input(self, value: object) -> None:
        from app.harness.scene_analysis_schemas import SceneOccurrenceBindingInput

        if not isinstance(value, SceneOccurrenceBindingInput):
            raise ValueError("Scene occurrence binding input is invalid")
        if (
            self.source_version_id != value.source_version_id
            or self.source_hash != value.source_hash
            or self.structure_identity_set_version_id != value.structure_identity_set_version_id
            or self.structure_identity_set_version_hash != value.structure_identity_set_version_hash
            or self.scene_fact_candidate_revision_id != value.scene_fact_candidate_revision_id
            or self.scene_fact_candidate_revision_hash != value.scene_fact_candidate_revision_hash
            or self.production_entity_candidate_revision_id
            != value.production_entity_candidate_revision_id
            or self.production_entity_candidate_revision_hash
            != value.production_entity_candidate_revision_hash
        ):
            raise ValueError("Scene binding lineage drifted")

        facts = SceneFactCandidate.model_validate(value.scene_fact_candidate)
        production = ProductionEntityFragmentCandidate.model_validate(
            value.production_entity_candidate
        )
        formal_scenes = value.structure_identity_set.scene_refs
        if len(self.scenes) != len(formal_scenes):
            raise ValueError("Scene binding must cover every formal Scene")
        fact_by_key = {scene.temporary_scene_id: scene for scene in facts.scenes}
        entity_by_key = {entity.identity_key: entity for entity in production.entities}
        mappings_by_scene: dict[str, list[FrozenStructureIdentityMentionMapping]] = {}
        for mapping in value.structure_identity_set.mention_mappings:
            if mapping.resolution == "resolved":
                mappings_by_scene.setdefault(mapping.temporary_scene_id, []).append(mapping)

        for supplied, formal in zip(self.scenes, formal_scenes, strict=True):
            if (
                supplied.scene_scope_key != formal.scope_key
                or str(supplied.scene_owner_logical_id) != str(formal.scene_owner_logical_id)
                or supplied.temporary_scene_id != formal.temporary_scene_id
                or supplied.source_start != formal.source_start
                or supplied.source_end != formal.source_end
            ):
                raise ValueError("Scene binding does not match its formal Scene")
            fact = fact_by_key[formal.temporary_scene_id]
            if len(supplied.beats) != len(fact.actions) or len(supplied.dialogues) != len(
                fact.dialogues
            ):
                raise ValueError("Scene binding changed the frozen Beat or Dialogue set")
            for beat, action in zip(supplied.beats, fact.actions, strict=True):
                if beat.text != action.text or beat.evidence != action.evidence:
                    raise ValueError("Scene Beat does not match frozen SceneFacts")
            formal_character_mappings = {
                (
                    mapping.identity_key,
                    mapping.source_start,
                    mapping.source_end,
                    mapping.text_hash,
                    mapping.exact_anchor,
                )
                for mapping in mappings_by_scene.get(formal.temporary_scene_id, [])
                if mapping.kind == "character"
            }
            for dialogue, fact_dialogue in zip(supplied.dialogues, fact.dialogues, strict=True):
                if (
                    dialogue.text != fact_dialogue.text
                    or dialogue.evidence != fact_dialogue.evidence
                ):
                    raise ValueError("Scene Dialogue does not match frozen SceneFacts")
                if dialogue.speaker_identity_key is not None:
                    assert dialogue.speaker_evidence is not None
                    speaker = (
                        dialogue.speaker_identity_key,
                        dialogue.speaker_evidence.source_start,
                        dialogue.speaker_evidence.source_end,
                        dialogue.speaker_evidence.text_hash,
                        dialogue.speaker_evidence.exact_anchor,
                    )
                    if (
                        speaker not in formal_character_mappings
                        or dialogue.speaker_evidence.exact_anchor != fact_dialogue.speaker_mention
                    ):
                        raise ValueError("Dialogue speaker is not an exact formal identity mapping")

            expected_mappings = sorted(
                mappings_by_scene.get(formal.temporary_scene_id, []),
                key=lambda item: (
                    item.source_start,
                    item.source_end,
                    item.kind,
                    "" if item.identity_key is None else item.identity_key,
                ),
            )
            if len(supplied.occurrences) != len(expected_mappings):
                raise ValueError("Scene occurrences do not cover the resolved mention partition")
            for occurrence, mapping in zip(supplied.occurrences, expected_mappings, strict=True):
                entity = entity_by_key.get(occurrence.identity_key)
                evidence = occurrence.evidence
                if (
                    mapping.identity_key is None
                    or occurrence.subject_kind != mapping.kind
                    or occurrence.identity_key != mapping.identity_key
                    or occurrence.occurrence_role != mapping.occurrence_role
                    or (
                        evidence.source_start,
                        evidence.source_end,
                        evidence.text_hash,
                        evidence.exact_anchor,
                    )
                    != (
                        mapping.source_start,
                        mapping.source_end,
                        mapping.text_hash,
                        mapping.exact_anchor,
                    )
                    or entity is None
                    or not any(
                        state.state_key == occurrence.state_key
                        and formal.scope_key in state.applicable_scene_scope_keys
                        for state in entity.states
                    )
                ):
                    raise ValueError(
                        "Scene occurrence is not an exact formal identity/state binding"
                    )
        for issue in self.review_issues:
            for evidence in issue.evidence:
                evidence.validate_for_text(value.normalized_text)
                if _source_evidence_key(evidence) not in value.scene_fact_evidence_universe():
                    raise ValueError("Scene binding review Evidence is outside frozen SceneFacts")


class PositiveRational(StrictSceneAnalysisModel):
    numerator: int = Field(ge=1, le=9_007_199_254_740_991)
    denominator: int = Field(ge=1, le=9_007_199_254_740_991)

    @model_validator(mode="after")
    def validate_reduced(self) -> PositiveRational:
        if math.gcd(self.numerator, self.denominator) != 1:
            raise ValueError("relative scale must be a reduced positive rational")
        return self


class SceneStoryTimeFragment(StrictSceneAnalysisModel):
    scene_scope_key: str = Field(pattern=r"^scene:[0-9a-f-]{36}$")
    story_time_key: str = Field(pattern=r"^storytime:[a-z0-9][a-z0-9_.:-]{0,127}$")


class InteractionGeometryEvidence(StrictSceneAnalysisModel):
    hand: SourceEvidenceSpan | None
    grip_type: SourceEvidenceSpan | None
    contact_point: SourceEvidenceSpan | None
    direction: SourceEvidenceSpan | None
    relative_scale: SourceEvidenceSpan | None

    def supplied(self) -> list[SourceEvidenceSpan]:
        return [
            value
            for value in (
                self.hand,
                self.grip_type,
                self.contact_point,
                self.direction,
                self.relative_scale,
            )
            if value is not None
        ]


class InteractionFragment(StrictSceneAnalysisModel):
    interaction_key: str = Field(pattern=r"^interaction_[a-z0-9_]{1,120}$")
    claim_series_key: str = Field(pattern=r"^interaction_series_[a-z0-9_]{1,120}$")
    claim_revision: int = Field(ge=1, le=9_007_199_254_740_991)
    supersedes_interaction_key: str | None
    scene_scope_key: str = Field(pattern=r"^scene:[0-9a-f-]{36}$")
    beat_key: str | None
    story_time_key: str = Field(pattern=r"^storytime:[a-z0-9][a-z0-9_.:-]{0,127}$")
    predicate: Literal[
        "hold", "carry", "wear", "use", "give", "receive", "place", "drop", "open", "break"
    ]
    actor_occurrence_key: str = Field(pattern=r"^occurrence_[a-z0-9_]{1,120}$")
    prop_occurrence_key: str = Field(pattern=r"^occurrence_[a-z0-9_]{1,120}$")
    counterparty_occurrence_key: str | None
    holder_before_identity_key: str | None
    holder_after_identity_key: str | None
    prop_state_before_key: str = Field(pattern=r"^state_[a-z0-9_]{1,120}$")
    prop_state_after_key: str = Field(pattern=r"^state_[a-z0-9_]{1,120}$")
    state_delta: str | None
    hand: Literal["left", "right", "both", "unspecified"]
    grip_type: str | None
    contact_point: str | None
    direction: str | None
    relative_scale: PositiveRational | None
    geometry_evidence: InteractionGeometryEvidence
    evidence: SourceEvidenceSpan

    @model_validator(mode="after")
    def validate_claim_and_delta(self) -> InteractionFragment:
        if (self.claim_revision == 1) != (self.supersedes_interaction_key is None):
            raise ValueError("interaction claim revision and supersedes key must form one chain")
        if (
            self.supersedes_interaction_key is not None
            and re.fullmatch(r"interaction_[a-z0-9_]{1,120}", self.supersedes_interaction_key)
            is None
        ):
            raise ValueError("interaction supersedes key is invalid")
        for value in (self.grip_type, self.contact_point, self.direction, self.state_delta):
            if value is not None and (not value.strip() or value != value.strip()):
                raise ValueError("interaction descriptors must be non-empty canonical strings")
        changed = self.prop_state_before_key != self.prop_state_after_key
        if changed != (self.state_delta is not None):
            raise ValueError("Prop state changes require exactly one explicit delta")
        if (self.hand != "unspecified") != (self.geometry_evidence.hand is not None):
            raise ValueError("specified hand requires exact geometry Evidence")
        for descriptor, evidence in (
            (self.grip_type, self.geometry_evidence.grip_type),
            (self.contact_point, self.geometry_evidence.contact_point),
            (self.direction, self.geometry_evidence.direction),
            (self.relative_scale, self.geometry_evidence.relative_scale),
        ):
            if (descriptor is not None) != (evidence is not None):
                raise ValueError("geometry fields and their Evidence must be supplied together")
        return self


class ContinuityFragment(StrictSceneAnalysisModel):
    continuity_key: str = Field(pattern=r"^continuity_[a-z0-9_]{1,120}$")
    claim_series_key: str = Field(pattern=r"^continuity_series_[a-z0-9_]{1,120}$")
    claim_revision: int = Field(ge=1, le=9_007_199_254_740_991)
    supersedes_continuity_key: str | None
    subject_kind: Literal["character", "location", "prop"]
    identity_key: str = Field(min_length=1)
    from_scene_scope_key: str = Field(pattern=r"^scene:[0-9a-f-]{36}$")
    to_scene_scope_key: str = Field(pattern=r"^scene:[0-9a-f-]{36}$")
    story_time_start: str = Field(pattern=r"^storytime:[a-z0-9][a-z0-9_.:-]{0,127}$")
    story_time_end: str = Field(pattern=r"^storytime:[a-z0-9][a-z0-9_.:-]{0,127}$")
    before_state_key: str = Field(pattern=r"^state_[a-z0-9_]{1,120}$")
    after_state_key: str = Field(pattern=r"^state_[a-z0-9_]{1,120}$")
    transition: Literal["state_persists", "state_changes"]
    delta: str | None
    evidence: list[SourceEvidenceSpan] = Field(min_length=1)

    @model_validator(mode="after")
    def validate_transition(self) -> ContinuityFragment:
        if (self.claim_revision == 1) != (self.supersedes_continuity_key is None):
            raise ValueError("continuity claim revision and supersedes key must form one chain")
        if (
            self.supersedes_continuity_key is not None
            and re.fullmatch(r"continuity_[a-z0-9_]{1,120}", self.supersedes_continuity_key) is None
        ):
            raise ValueError("continuity supersedes key is invalid")
        if self.story_time_start >= self.story_time_end:
            raise ValueError("continuity story time must be strictly increasing")
        if self.transition == "state_persists":
            if self.before_state_key != self.after_state_key or self.delta is not None:
                raise ValueError("state_persists must preserve the exact state without a delta")
        elif self.before_state_key == self.after_state_key or not self.delta:
            raise ValueError("state_changes must bind distinct states and a delta")
        return self


class ContinuityLedgerEntry(StrictSceneAnalysisModel):
    ledger_key: str = Field(pattern=r"^ledger_[a-z0-9_]{1,120}$")
    subject_kind: Literal["character", "prop"]
    identity_key: str = Field(min_length=1)
    scene_scope_key: str = Field(pattern=r"^scene:[0-9a-f-]{36}$")
    story_time_key: str = Field(pattern=r"^storytime:[a-z0-9][a-z0-9_.:-]{0,127}$")
    state_key: str = Field(pattern=r"^state_[a-z0-9_]{1,120}$")
    holder_identity_key: str | None
    location_identity_key: str | None
    transition_interaction_key: str | None
    evidence: list[SourceEvidenceSpan] = Field(min_length=1)

    @model_validator(mode="after")
    def validate_shape(self) -> ContinuityLedgerEntry:
        if self.subject_kind == "character" and (
            self.holder_identity_key is not None or self.transition_interaction_key is not None
        ):
            raise ValueError("character ledger entries cannot carry holder or Prop transition")
        if (
            self.transition_interaction_key is not None
            and re.fullmatch(r"interaction_[a-z0-9_]{1,120}", self.transition_interaction_key)
            is None
        ):
            raise ValueError("ledger transition Interaction key is invalid")
        evidence_keys = [_source_evidence_key(value) for value in self.evidence]
        if len(evidence_keys) != len(set(evidence_keys)):
            raise ValueError("ledger Evidence must be unique")
        return self


class InteractionContinuityCandidate(StrictSceneAnalysisModel):
    source_version_id: UUID
    source_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    structure_identity_set_version_id: UUID
    structure_identity_set_version_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    scene_fact_candidate_revision_id: UUID
    scene_fact_candidate_revision_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    production_entity_candidate_revision_id: UUID
    production_entity_candidate_revision_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    scene_binding_candidate_revision_id: UUID
    scene_binding_candidate_revision_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    scene_story_times: list[SceneStoryTimeFragment] = Field(min_length=1)
    interactions: list[InteractionFragment]
    continuity_ledger: list[ContinuityLedgerEntry] = Field(min_length=1)
    continuity: list[ContinuityFragment]
    review_issues: list[CandidateReviewIssue]

    def validate_for_input(self, value: object) -> None:
        from app.harness.scene_analysis_schemas import InteractionContinuityInput

        if not isinstance(value, InteractionContinuityInput):
            raise ValueError("interaction continuity input is invalid")
        if (
            self.source_version_id != value.source_version_id
            or self.source_hash != value.source_hash
            or self.structure_identity_set_version_id != value.structure_identity_set_version_id
            or self.structure_identity_set_version_hash != value.structure_identity_set_version_hash
            or self.scene_fact_candidate_revision_id != value.scene_fact_candidate_revision_id
            or self.scene_fact_candidate_revision_hash != value.scene_fact_candidate_revision_hash
            or self.production_entity_candidate_revision_id
            != value.production_entity_candidate_revision_id
            or self.production_entity_candidate_revision_hash
            != value.production_entity_candidate_revision_hash
            or self.scene_binding_candidate_revision_id != value.scene_binding_candidate_revision_id
            or self.scene_binding_candidate_revision_hash
            != value.scene_binding_candidate_revision_hash
        ):
            raise ValueError("interaction continuity lineage drifted")

        bindings = SceneBindingFragmentCandidate.model_validate(value.scene_binding_candidate)
        production = ProductionEntityFragmentCandidate.model_validate(
            value.production_entity_candidate
        )
        facts = SceneFactCandidate.model_validate(value.scene_fact_candidate)
        scenes = {scene.scene_scope_key: scene for scene in bindings.scenes}
        expected_scene_keys = {scene.scene_scope_key for scene in bindings.scenes}
        supplied_scene_keys = {item.scene_scope_key for item in self.scene_story_times}
        story_time_by_scene = {
            item.scene_scope_key: item.story_time_key for item in self.scene_story_times
        }
        if (
            supplied_scene_keys != expected_scene_keys
            or len(supplied_scene_keys) != len(self.scene_story_times)
            or len(set(story_time_by_scene.values())) != len(self.scene_story_times)
            or self.scene_story_times
            != sorted(self.scene_story_times, key=lambda item: item.story_time_key)
        ):
            raise ValueError(
                "story-time anchors must cover every Scene once in chronological order"
            )
        occurrences = {
            occurrence.occurrence_key: (scene.scene_scope_key, occurrence)
            for scene in bindings.scenes
            for occurrence in scene.occurrences
        }
        states = {
            state.state_key: (entity.identity_key, entity.kind)
            for entity in production.entities
            for state in entity.states
        }
        action_evidence = {
            scene_ref.scope_key: {
                _source_evidence_key(action.evidence)
                for action in next(
                    item
                    for item in facts.scenes
                    if item.temporary_scene_id == scene_ref.temporary_scene_id
                ).actions
            }
            for scene_ref in value.structure_identity_set.scene_refs
        }
        if len({item.interaction_key for item in self.interactions}) != len(self.interactions):
            raise ValueError("interaction keys must be unique")
        if len({item.claim_series_key for item in self.interactions}) != len(self.interactions):
            raise ValueError("interaction claim series must be unique")
        if len({item.continuity_key for item in self.continuity}) != len(self.continuity):
            raise ValueError("continuity keys must be unique")
        if len({item.claim_series_key for item in self.continuity}) != len(self.continuity):
            raise ValueError("continuity claim series must be unique")
        if self.interactions != sorted(
            self.interactions, key=lambda item: (item.story_time_key, item.interaction_key)
        ):
            raise ValueError("interactions must be in canonical story-time order")
        if self.continuity != sorted(
            self.continuity,
            key=lambda item: (item.story_time_start, item.story_time_end, item.continuity_key),
        ):
            raise ValueError("continuity must be in canonical story-time order")
        if self.continuity_ledger != sorted(
            self.continuity_ledger,
            key=lambda item: (item.story_time_key, item.subject_kind, item.identity_key),
        ):
            raise ValueError("continuity ledger must be in canonical story-time order")

        prop_ledger: dict[str, tuple[str | None, str, str]] = {}
        for item in self.interactions:
            scene = scenes.get(item.scene_scope_key)
            actor = occurrences.get(item.actor_occurrence_key)
            prop = occurrences.get(item.prop_occurrence_key)
            counterparty = (
                occurrences.get(item.counterparty_occurrence_key)
                if item.counterparty_occurrence_key is not None
                else None
            )
            if (
                scene is None
                or actor is None
                or prop is None
                or actor[0] != item.scene_scope_key
                or prop[0] != item.scene_scope_key
                or actor[1].subject_kind != "character"
                or prop[1].subject_kind != "prop"
                or actor[1].occurrence_role != "actual"
                or prop[1].occurrence_role != "actual"
                or item.story_time_key != story_time_by_scene.get(item.scene_scope_key)
                or item.beat_key is not None
                and item.beat_key not in {beat.beat_key for beat in scene.beats}
                or states.get(item.prop_state_before_key) != (prop[1].identity_key, "prop")
                or states.get(item.prop_state_after_key) != (prop[1].identity_key, "prop")
                or _source_evidence_key(item.evidence) not in action_evidence[item.scene_scope_key]
                or any(
                    _source_evidence_key(evidence) not in action_evidence[item.scene_scope_key]
                    for evidence in item.geometry_evidence.supplied()
                )
            ):
                raise ValueError(
                    "interaction does not bind exact actual occurrences and Prop states"
                )
            participant_identities = {actor[1].identity_key}
            if item.predicate in {"give", "receive"}:
                if (
                    counterparty is None
                    or counterparty[0] != item.scene_scope_key
                    or counterparty[1].subject_kind != "character"
                    or counterparty[1].occurrence_role != "actual"
                    or counterparty[1].identity_key == actor[1].identity_key
                ):
                    raise ValueError(
                        "transfer interaction requires one distinct actual counterparty"
                    )
                participant_identities.add(counterparty[1].identity_key)
            elif counterparty is not None:
                raise ValueError("non-transfer interaction cannot add a counterparty")
            if item.holder_before_identity_key not in participant_identities | {
                None
            } or item.holder_after_identity_key not in participant_identities | {None}:
                raise ValueError("interaction holder must be null or one participant")
            actor_identity = actor[1].identity_key
            counterparty_identity = None if counterparty is None else counterparty[1].identity_key
            before_holder, after_holder = (
                item.holder_before_identity_key,
                item.holder_after_identity_key,
            )
            if item.predicate == "hold" and not (
                before_holder in {None, actor_identity} and after_holder == actor_identity
            ):
                raise ValueError("hold transition is invalid")
            if item.predicate == "carry" and not (
                before_holder == actor_identity and after_holder == actor_identity
            ):
                raise ValueError("carry transition is invalid")
            if item.predicate == "wear" and not (
                before_holder in {None, actor_identity}
                and after_holder == actor_identity
                and (before_holder is not None or item.state_delta is not None)
            ):
                raise ValueError("wear transition is invalid")
            if item.predicate == "use" and before_holder != after_holder:
                raise ValueError("use cannot implicitly transfer a holder")
            if item.predicate == "give" and not (
                before_holder == actor_identity and after_holder == counterparty_identity
            ):
                raise ValueError("give transition is invalid")
            if item.predicate == "receive" and not (
                before_holder == counterparty_identity and after_holder == actor_identity
            ):
                raise ValueError("receive transition is invalid")
            if item.predicate in {"place", "drop"} and not (
                before_holder == actor_identity
                and after_holder is None
                and item.state_delta is not None
            ):
                raise ValueError("release transition is invalid")
            if item.predicate in {"open", "break"} and not (
                before_holder == after_holder and item.state_delta is not None
            ):
                raise ValueError("Prop mutation transition is invalid")

            prop_identity = prop[1].identity_key
            previous_prop_transition = prop_ledger.get(prop_identity)
            if previous_prop_transition is not None and (
                previous_prop_transition[2] == item.story_time_key
                or previous_prop_transition[0] != before_holder
                or previous_prop_transition[1] != item.prop_state_before_key
            ):
                raise ValueError("Prop ledger has a duplicate or discontinuous transition")
            prop_ledger[prop_identity] = (
                after_holder,
                item.prop_state_after_key,
                item.story_time_key,
            )

        interaction_by_key = {item.interaction_key: item for item in self.interactions}
        actual_subjects: dict[tuple[str, str], SceneOccurrenceFragment] = {}
        actual_locations: dict[str, dict[str, SceneOccurrenceFragment]] = {}
        for scene_key, occurrence in occurrences.values():
            if occurrence.occurrence_role != "actual":
                continue
            if occurrence.subject_kind in {"character", "prop"}:
                subject_key = (scene_key, occurrence.identity_key)
                previous_occurrence = actual_subjects.get(subject_key)
                if (
                    previous_occurrence is not None
                    and previous_occurrence.state_key != occurrence.state_key
                ):
                    raise ValueError("one Scene cannot bind two ledger states for one identity")
                actual_subjects[subject_key] = occurrence
            elif occurrence.subject_kind == "location":
                actual_locations.setdefault(scene_key, {})[occurrence.identity_key] = occurrence

        supplied_subjects: dict[tuple[str, str], ContinuityLedgerEntry] = {}
        prop_entries: dict[str, list[ContinuityLedgerEntry]] = {}
        used_interactions: set[str] = set()
        ledger_keys: set[str] = set()
        evidence_universe = value.scene_fact_evidence_universe()
        for entry in self.continuity_ledger:
            subject_key = (entry.scene_scope_key, entry.identity_key)
            occurrence = actual_subjects.get(subject_key)
            scene_locations = actual_locations.get(entry.scene_scope_key, {})
            location = (
                None
                if entry.location_identity_key is None
                else scene_locations.get(entry.location_identity_key)
            )
            evidence_keys = {_source_evidence_key(evidence) for evidence in entry.evidence}
            if (
                entry.ledger_key in ledger_keys
                or subject_key in supplied_subjects
                or occurrence is None
                or occurrence.subject_kind != entry.subject_kind
                or occurrence.state_key != entry.state_key
                or bool(scene_locations) != (location is not None)
                or entry.story_time_key != story_time_by_scene.get(entry.scene_scope_key)
                or _source_evidence_key(occurrence.evidence) not in evidence_keys
                or location is not None
                and _source_evidence_key(location.evidence) not in evidence_keys
                or not evidence_keys.issubset(evidence_universe)
            ):
                raise ValueError(
                    "continuity ledger does not bind exact subject, state, location, and Evidence"
                )
            ledger_keys.add(entry.ledger_key)
            supplied_subjects[subject_key] = entry
            if entry.subject_kind == "prop":
                if entry.holder_identity_key is not None:
                    holder = actual_subjects.get((entry.scene_scope_key, entry.holder_identity_key))
                    if holder is None or holder.subject_kind != "character":
                        raise ValueError("Prop ledger holder must be an actual Character")
                prop_entries.setdefault(entry.identity_key, []).append(entry)
            if entry.transition_interaction_key is not None:
                transition = interaction_by_key.get(entry.transition_interaction_key)
                if (
                    transition is None
                    or entry.transition_interaction_key in used_interactions
                    or transition.scene_scope_key != entry.scene_scope_key
                    or transition.story_time_key != entry.story_time_key
                    or occurrences[transition.prop_occurrence_key][1].identity_key
                    != entry.identity_key
                    or transition.prop_state_after_key != entry.state_key
                    or transition.holder_after_identity_key != entry.holder_identity_key
                ):
                    raise ValueError("ledger transition does not match its Interaction exit state")
                used_interactions.add(entry.transition_interaction_key)
        if set(supplied_subjects) != set(actual_subjects):
            raise ValueError("continuity ledger must cover every actual Character and Prop once")
        if used_interactions != set(interaction_by_key):
            raise ValueError("every Interaction must produce exactly one Prop ledger transition")

        for entries in prop_entries.values():
            previous_entry: ContinuityLedgerEntry | None = None
            for entry in entries:
                if previous_entry is not None:
                    transition = (
                        None
                        if entry.transition_interaction_key is None
                        else interaction_by_key[entry.transition_interaction_key]
                    )
                    if transition is None:
                        if (
                            entry.state_key != previous_entry.state_key
                            or entry.holder_identity_key != previous_entry.holder_identity_key
                            or entry.location_identity_key != previous_entry.location_identity_key
                        ):
                            raise ValueError(
                                "Prop ledger contains an unexplained state or teleport"
                            )
                    elif (
                        transition.prop_state_before_key != previous_entry.state_key
                        or transition.holder_before_identity_key
                        != previous_entry.holder_identity_key
                        or entry.location_identity_key != previous_entry.location_identity_key
                        and transition.predicate not in {"carry", "give", "receive"}
                    ):
                        raise ValueError(
                            "Prop ledger transition does not explain its boundary change"
                        )
                previous_entry = entry

        continuity_ledger: dict[str, tuple[str, str]] = {}
        for item in self.continuity:
            before = [
                occurrence
                for scene_key, occurrence in occurrences.values()
                if scene_key == item.from_scene_scope_key
                and occurrence.identity_key == item.identity_key
                and occurrence.occurrence_role == "actual"
            ]
            after = [
                occurrence
                for scene_key, occurrence in occurrences.values()
                if scene_key == item.to_scene_scope_key
                and occurrence.identity_key == item.identity_key
                and occurrence.occurrence_role == "actual"
            ]
            if (
                not before
                or not after
                or item.story_time_start != story_time_by_scene.get(item.from_scene_scope_key)
                or item.story_time_end != story_time_by_scene.get(item.to_scene_scope_key)
                or states.get(item.before_state_key) != (item.identity_key, item.subject_kind)
                or states.get(item.after_state_key) != (item.identity_key, item.subject_kind)
                or any(
                    _source_evidence_key(evidence) not in value.scene_fact_evidence_universe()
                    for evidence in item.evidence
                )
            ):
                raise ValueError(
                    "continuity does not bind an ordered exact identity/state timeline"
                )
            previous_continuity = continuity_ledger.get(item.identity_key)
            if previous_continuity is not None and (
                previous_continuity[0] > item.story_time_start
                or previous_continuity[1] != item.before_state_key
            ):
                raise ValueError("continuity ledger overlaps or contains an unexplained state jump")
            continuity_ledger[item.identity_key] = (item.story_time_end, item.after_state_key)

        character_entries: dict[str, list[ContinuityLedgerEntry]] = {}
        for entry in self.continuity_ledger:
            if entry.subject_kind == "character":
                character_entries.setdefault(entry.identity_key, []).append(entry)
        continuity_links = {
            (
                item.identity_key,
                item.from_scene_scope_key,
                item.to_scene_scope_key,
                item.before_state_key,
                item.after_state_key,
            )
            for item in self.continuity
            if item.subject_kind == "character"
        }
        for entries in character_entries.values():
            for before, after in zip(entries, entries[1:], strict=False):
                if (
                    before.identity_key,
                    before.scene_scope_key,
                    after.scene_scope_key,
                    before.state_key,
                    after.state_key,
                ) not in continuity_links:
                    raise ValueError("Character ledger boundary lacks an exact Continuity claim")
        for issue in self.review_issues:
            for evidence in issue.evidence:
                if _source_evidence_key(evidence) not in value.scene_fact_evidence_universe():
                    raise ValueError("continuity review Evidence is outside frozen SceneFacts")

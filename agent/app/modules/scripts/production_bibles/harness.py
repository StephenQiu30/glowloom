from __future__ import annotations

import hashlib
import json
from dataclasses import dataclass
from datetime import UTC, datetime
from typing import Any, Literal, Protocol
from uuid import UUID

from langchain_core.messages import HumanMessage, SystemMessage
from pydantic import BaseModel, ConfigDict, Field, model_validator

from app.modules.scripts.documents.analysis import analyze_document
from app.modules.scripts.production_bibles.ports import (
    PRODUCTION_BIBLE_HARNESS_VERSION,
    ProductionBibleInput,
)
from app.modules.scripts.production_bibles.schemas import (
    BibleEvidence,
    BibleEvidenceChunkResult,
    BibleReviewIssue,
    ProductionBibleProviderResult,
    ProductionBibleReviewResult,
)
from app.modules.skills import StructuredSkillModel

EVIDENCE_CHUNK_MAX_CHARS = 18_000


class _HarnessModel(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)


class ProductionBibleCheckpoint(_HarnessModel):
    bible_id: UUID
    task_id: UUID
    run_token: UUID
    input_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    harness_version: str
    stage: Literal["evidence", "reconciled", "reviewed"]
    completed_chunk_keys: tuple[str, ...] = ()
    evidence_chunks: tuple[BibleEvidenceChunkResult, ...] = ()
    candidate: ProductionBibleProviderResult | None = None
    review: ProductionBibleReviewResult | None = None
    updated_at: datetime

    @model_validator(mode="after")
    def validate_stage_payload(self) -> ProductionBibleCheckpoint:
        chunk_keys = tuple(chunk.chunk_key for chunk in self.evidence_chunks)
        if chunk_keys != self.completed_chunk_keys:
            raise ValueError("completed chunk keys must match stored evidence chunks")
        if len(set(chunk_keys)) != len(chunk_keys):
            raise ValueError("checkpoint chunk keys must be unique")
        if self.stage in {"reconciled", "reviewed"} and self.candidate is None:
            raise ValueError("reconciled checkpoints require a candidate")
        if self.stage == "reviewed" and self.review is None:
            raise ValueError("reviewed checkpoints require a review")
        return self


class ProductionBibleCheckpointStore(Protocol):
    async def load_latest(
        self,
        bible_id: UUID,
        input_hash: str,
    ) -> ProductionBibleCheckpoint | None: ...

    async def save(self, checkpoint: ProductionBibleCheckpoint) -> None: ...


@dataclass(frozen=True, slots=True)
class ProductionBibleAgentModels:
    evidence: StructuredSkillModel
    reconcile: StructuredSkillModel
    review: StructuredSkillModel


@dataclass(frozen=True, slots=True)
class _EvidenceChunk:
    key: str
    source_start: int
    source_end: int
    catalog: tuple[BibleEvidence, ...]


def _episode_for_range(
    source_start: int,
    source_end: int,
    markers: tuple[Any, ...],
) -> int | None:
    overlapping = next(
        (
            marker.episode_number
            for marker in markers
            if marker.start_codepoint < source_end and marker.end_codepoint > source_start
        ),
        None,
    )
    if overlapping is not None:
        return overlapping
    current: int | None = None
    for marker in markers:
        if marker.start_codepoint > source_start:
            break
        current = marker.episode_number
    return current


def _atomic_evidence_catalog(script_text: str) -> tuple[BibleEvidence, ...]:
    analysis = analyze_document(script_text)
    evidence: list[BibleEvidence] = []
    for block in analysis.blocks:
        start = block.start_codepoint
        while start < block.end_codepoint:
            end = min(start + EVIDENCE_CHUNK_MAX_CHARS, block.end_codepoint)
            anchor = script_text[start:end]
            evidence.append(
                BibleEvidence(
                    source_start=start,
                    source_end=end,
                    text_hash=hashlib.sha256(anchor.encode("utf-8")).hexdigest(),
                    exact_anchor=anchor,
                    episode_number=_episode_for_range(
                        start,
                        end,
                        analysis.markers,
                    ),
                )
            )
            start = end
    return tuple(evidence)


def build_evidence_chunks(script_text: str) -> tuple[_EvidenceChunk, ...]:
    catalog = _atomic_evidence_catalog(script_text)
    chunks: list[_EvidenceChunk] = []
    current: list[BibleEvidence] = []
    current_size = 0
    for item in catalog:
        item_size = item.source_end - item.source_start
        if current and current_size + item_size > EVIDENCE_CHUNK_MAX_CHARS:
            chunks.append(
                _EvidenceChunk(
                    key=f"chunk-{len(chunks) + 1:04d}",
                    source_start=current[0].source_start,
                    source_end=current[-1].source_end,
                    catalog=tuple(current),
                )
            )
            current = []
            current_size = 0
        current.append(item)
        current_size += item_size
    if current:
        chunks.append(
            _EvidenceChunk(
                key=f"chunk-{len(chunks) + 1:04d}",
                source_start=current[0].source_start,
                source_end=current[-1].source_end,
                catalog=tuple(current),
            )
        )
    return tuple(chunks)


def _evidence_key(evidence: BibleEvidence) -> tuple[int, int, str]:
    return evidence.source_start, evidence.source_end, evidence.text_hash


def _build_reconcile_payload(
    *,
    document_revision_id: UUID,
    input_hash: str,
    evidence_chunks: tuple[BibleEvidenceChunkResult, ...],
) -> dict[str, object]:
    unique_evidence: dict[tuple[int, int, str], BibleEvidence] = {}
    for chunk in evidence_chunks:
        for observation in chunk.observations:
            for evidence in observation.evidence:
                identity = _evidence_key(evidence)
                existing = unique_evidence.get(identity)
                if existing is not None and existing != evidence:
                    raise ValueError("validated observations contain conflicting evidence")
                unique_evidence[identity] = evidence

    evidence_refs = {
        identity: f"e{index:04d}" for index, identity in enumerate(sorted(unique_evidence), start=1)
    }
    evidence_catalog = {
        evidence_refs[identity]: unique_evidence[identity].model_dump(mode="json")
        for identity in sorted(unique_evidence)
    }
    observations: list[dict[str, object]] = []
    for chunk in evidence_chunks:
        for observation in chunk.observations:
            compact_observation: dict[str, object] = observation.model_dump(
                mode="json",
                exclude={"evidence"},
            )
            compact_observation["chunk_key"] = chunk.chunk_key
            compact_observation["evidence_refs"] = list(
                dict.fromkeys(
                    evidence_refs[_evidence_key(evidence)] for evidence in observation.evidence
                )
            )
            observations.append(compact_observation)

    return {
        "document_revision_id": str(document_revision_id),
        "input_hash": input_hash,
        "evidence_catalog": evidence_catalog,
        "observations": observations,
    }


def _validate_chunk_result(
    result: BibleEvidenceChunkResult,
    chunk: _EvidenceChunk,
) -> None:
    if (
        result.chunk_key != chunk.key
        or result.source_start != chunk.source_start
        or result.source_end != chunk.source_end
    ):
        raise ValueError("evidence result does not match its immutable chunk")
    allowed = {_evidence_key(item): item for item in chunk.catalog}
    for observation in result.observations:
        for evidence in observation.evidence:
            expected = allowed.get(_evidence_key(evidence))
            if expected is None or evidence != expected:
                raise ValueError("evidence result cites a span outside the supplied catalog")


def _all_candidate_evidence(
    result: ProductionBibleProviderResult,
) -> tuple[BibleEvidence, ...]:
    return tuple(
        [evidence for entity in result.entities for evidence in entity.evidence]
        + [
            evidence
            for entity in result.entities
            for state in entity.states
            for evidence in state.evidence
        ]
        + [evidence for entry in result.world_entries for evidence in entry.evidence]
        + [evidence for issue in result.review_issues for evidence in issue.evidence]
    )


def _deterministic_review_issues(
    result: ProductionBibleProviderResult,
) -> tuple[BibleReviewIssue, ...]:
    owners: dict[tuple[str, str], tuple[str, tuple[BibleEvidence, ...]]] = {}
    issues: list[BibleReviewIssue] = []
    for entity in result.entities:
        identity_names = (entity.canonical_name, *entity.aliases)
        for name in identity_names:
            normalized = " ".join(name.casefold().split())
            key = entity.kind, normalized
            previous = owners.get(key)
            if previous is None:
                owners[key] = (entity.entity_key, tuple(entity.evidence))
                continue
            previous_key, previous_evidence = previous
            if previous_key == entity.entity_key:
                continue
            issues.append(
                BibleReviewIssue(
                    issue_key=f"alias-collision:{entity.kind}:{len(issues) + 1}",
                    code="bible.alias_collision",
                    severity="blocking",
                    scope="global",
                    subject_key=None,
                    summary=(
                        f"{name!r} is assigned to both {previous_key!r} and {entity.entity_key!r}"
                    ),
                    repair_hint="Merge the identities or remove the unsupported alias.",
                    evidence=list((*previous_evidence, *entity.evidence)[:100]),
                )
            )
    return tuple(issues)


def _validate_candidate(
    result: ProductionBibleProviderResult,
    evidence_chunks: tuple[BibleEvidenceChunkResult, ...],
) -> ProductionBibleProviderResult:
    allowed = {
        _evidence_key(evidence): evidence
        for chunk in evidence_chunks
        for observation in chunk.observations
        for evidence in observation.evidence
    }
    for evidence in _all_candidate_evidence(result):
        expected = allowed.get(_evidence_key(evidence))
        if expected is None or evidence != expected:
            raise ValueError("reconciled candidate introduced unsupported evidence")
    deterministic_issues = _deterministic_review_issues(result)
    issue_keys = {issue.issue_key for issue in result.review_issues}
    return result.model_copy(
        update={
            "review_issues": [
                *result.review_issues,
                *(issue for issue in deterministic_issues if issue.issue_key not in issue_keys),
            ]
        }
    )


def _merge_reviews(
    candidate: ProductionBibleProviderResult,
    review: ProductionBibleReviewResult,
) -> ProductionBibleProviderResult:
    issues: dict[str, BibleReviewIssue] = {
        issue.issue_key: issue for issue in candidate.review_issues
    }
    for issue in review.review_issues:
        issues.setdefault(issue.issue_key, issue)
    return candidate.model_copy(update={"review_issues": list(issues.values())})


class ProductionBibleAgentHarness:
    def __init__(
        self,
        *,
        models: ProductionBibleAgentModels,
        checkpoint_store: ProductionBibleCheckpointStore,
    ) -> None:
        self._models = models
        self._checkpoint_store = checkpoint_store

    async def run(
        self,
        bible_input: ProductionBibleInput,
    ) -> ProductionBibleProviderResult:
        chunks = build_evidence_chunks(bible_input.normalized_text)
        checkpoint = await self._checkpoint_store.load_latest(
            bible_input.bible_id,
            bible_input.input_hash,
        )
        if checkpoint is not None and (
            checkpoint.bible_id != bible_input.bible_id
            or checkpoint.task_id != bible_input.task_id
            or checkpoint.input_hash != bible_input.input_hash
            or checkpoint.harness_version != PRODUCTION_BIBLE_HARNESS_VERSION
        ):
            checkpoint = None
        if checkpoint is None:
            checkpoint = ProductionBibleCheckpoint(
                bible_id=bible_input.bible_id,
                task_id=bible_input.task_id,
                run_token=bible_input.run_token,
                input_hash=bible_input.input_hash,
                harness_version=PRODUCTION_BIBLE_HARNESS_VERSION,
                stage="evidence",
                updated_at=datetime.now(UTC),
            )
        elif checkpoint.run_token != bible_input.run_token:
            checkpoint = checkpoint.model_copy(update={"run_token": bible_input.run_token})

        if checkpoint.stage == "reviewed":
            assert checkpoint.candidate is not None and checkpoint.review is not None
            return _merge_reviews(checkpoint.candidate, checkpoint.review)

        completed = set(checkpoint.completed_chunk_keys)
        evidence_results = list(checkpoint.evidence_chunks)
        for chunk in chunks:
            if chunk.key in completed:
                continue
            raw = await self._models.evidence.ainvoke(
                [
                    SystemMessage(
                        content=(
                            "Extract only source-grounded local Production Bible observations. "
                            "Every evidence object must be copied unchanged from evidence_catalog."
                        )
                    ),
                    HumanMessage(
                        content=json.dumps(
                            {
                                "document_revision_id": str(bible_input.document_revision_id),
                                "input_hash": bible_input.input_hash,
                                "chunk_key": chunk.key,
                                "source_start": chunk.source_start,
                                "source_end": chunk.source_end,
                                "evidence_catalog": [
                                    item.model_dump(mode="json") for item in chunk.catalog
                                ],
                            },
                            ensure_ascii=False,
                            separators=(",", ":"),
                        )
                    ),
                ]
            )
            result = BibleEvidenceChunkResult.model_validate(raw)
            _validate_chunk_result(result, chunk)
            evidence_results.append(result)
            completed.add(chunk.key)
            checkpoint = checkpoint.model_copy(
                update={
                    "stage": "evidence",
                    "completed_chunk_keys": tuple(item.chunk_key for item in evidence_results),
                    "evidence_chunks": tuple(evidence_results),
                    "updated_at": datetime.now(UTC),
                }
            )
            await self._checkpoint_store.save(checkpoint)

        candidate = checkpoint.candidate
        if checkpoint.stage == "evidence" or candidate is None:
            raw_candidate = await self._models.reconcile.ainvoke(
                [
                    SystemMessage(
                        content=(
                            "Reconcile all validated whole-script observations into stable "
                            "identities, their time-varying states, and world entries. Input "
                            "observations cite short evidence_refs into evidence_catalog. In the "
                            "formal output, copy only the referenced evidence objects unchanged "
                            "and preserve ambiguity."
                        )
                    ),
                    HumanMessage(
                        content=json.dumps(
                            _build_reconcile_payload(
                                document_revision_id=bible_input.document_revision_id,
                                input_hash=bible_input.input_hash,
                                evidence_chunks=tuple(evidence_results),
                            ),
                            ensure_ascii=False,
                            separators=(",", ":"),
                        )
                    ),
                ]
            )
            candidate = _validate_candidate(
                ProductionBibleProviderResult.model_validate(raw_candidate),
                tuple(evidence_results),
            )
            checkpoint = checkpoint.model_copy(
                update={
                    "stage": "reconciled",
                    "candidate": candidate,
                    "updated_at": datetime.now(UTC),
                }
            )
            await self._checkpoint_store.save(checkpoint)

        raw_review = await self._models.review.ainvoke(
            [
                SystemMessage(
                    content=(
                        "Independently review the reconciled Bible for unsupported facts, "
                        "false merges or splits, alias collisions, and state/world conflicts. "
                        "Return issues only; do not rewrite the candidate."
                    )
                ),
                HumanMessage(
                    content=json.dumps(
                        {
                            "document_revision_id": str(bible_input.document_revision_id),
                            "input_hash": bible_input.input_hash,
                            "candidate": candidate.model_dump(mode="json"),
                            "validated_evidence": [
                                item.model_dump(mode="json") for item in evidence_results
                            ],
                        },
                        ensure_ascii=False,
                        separators=(",", ":"),
                    )
                ),
            ]
        )
        review = ProductionBibleReviewResult.model_validate(raw_review)
        reviewed = _merge_reviews(candidate, review)
        _validate_candidate(reviewed, tuple(evidence_results))
        checkpoint = checkpoint.model_copy(
            update={
                "stage": "reviewed",
                "review": review,
                "updated_at": datetime.now(UTC),
            }
        )
        await self._checkpoint_store.save(checkpoint)
        return reviewed

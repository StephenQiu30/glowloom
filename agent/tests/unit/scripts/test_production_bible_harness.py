import hashlib
import json
from collections.abc import Sequence
from dataclasses import replace
from typing import Any
from uuid import UUID

import pytest
from uuid6 import uuid7

from app.modules.scripts.production_bibles import (
    PRODUCTION_BIBLE_HARNESS_VERSION,
    ProductionBibleAgentHarness,
    ProductionBibleAgentModels,
    ProductionBibleCheckpoint,
    ProductionBibleInput,
    ProductionBibleProviderResult,
    build_evidence_chunks,
)


class _MemoryCheckpointStore:
    def __init__(self) -> None:
        self.latest: ProductionBibleCheckpoint | None = None
        self.saved: list[ProductionBibleCheckpoint] = []

    async def load_latest(
        self,
        bible_id: UUID,
        input_hash: str,
    ) -> ProductionBibleCheckpoint | None:
        if (
            self.latest is None
            or self.latest.bible_id != bible_id
            or self.latest.input_hash != input_hash
        ):
            return None
        return self.latest

    async def save(self, checkpoint: ProductionBibleCheckpoint) -> None:
        validated = ProductionBibleCheckpoint.model_validate(checkpoint.model_dump(mode="json"))
        self.latest = validated
        self.saved.append(validated)


class _EvidenceModel:
    def __init__(self) -> None:
        self.calls = 0

    async def ainvoke(self, messages: Sequence[Any]) -> object:
        self.calls += 1
        payload = json.loads(messages[-1].content)
        evidence = next(
            item for item in payload["evidence_catalog"] if "Aurelia" in item["exact_anchor"]
        )
        return {
            "chunk_key": payload["chunk_key"],
            "source_start": payload["source_start"],
            "source_end": payload["source_end"],
            "observations": [
                {
                    "observation_key": "aurelia-mention",
                    "kind": "entity",
                    "subject_key": "aurelia",
                    "parent_entity_key": None,
                    "claim": "Aurelia is named in the script.",
                    "evidence": [evidence],
                    "ambiguities": [],
                }
            ],
        }


class _ReconcileModel:
    def __init__(self) -> None:
        self.calls = 0

    async def ainvoke(self, messages: Sequence[Any]) -> object:
        self.calls += 1
        payload = json.loads(messages[-1].content)
        evidence_ref = payload["observations"][0]["evidence_refs"][0]
        evidence = payload["evidence_catalog"][evidence_ref]
        return {
            "entities": [
                {
                    "entity_key": "character:aurelia",
                    "kind": "character",
                    "canonical_name": "Aurelia",
                    "normalized_name": "aurelia",
                    "aliases": ["the Empress"],
                    "stable_spec": {"identity": "Aurelia"},
                    "episode_numbers": [1],
                    "evidence": [evidence],
                    "states": [],
                    "ambiguities": [],
                }
            ],
            "world_entries": [],
            "review_issues": [],
        }


class _ReviewModel:
    def __init__(self) -> None:
        self.calls = 0

    async def ainvoke(self, messages: Sequence[Any]) -> object:
        self.calls += 1
        assert "candidate" in json.loads(messages[-1].content)
        return {"review_issues": []}


def _input(script: str, *, run_token: UUID | None = None) -> ProductionBibleInput:
    return ProductionBibleInput(
        bible_id=uuid7(),
        task_id=uuid7(),
        workspace_id=uuid7(),
        project_id=uuid7(),
        document_revision_id=uuid7(),
        input_hash=hashlib.sha256(script.encode()).hexdigest(),
        normalized_text=script,
        run_token=run_token or uuid7(),
    )


def test_evidence_chunks_preserve_the_entire_document_coordinate_space() -> None:
    script = "EPISODE 1\nINT. THRONE ROOM - DAY\nAurelia enters.\n"

    chunks = build_evidence_chunks(script)

    assert chunks[0].source_start == 0
    assert chunks[-1].source_end == len(script)
    catalog = [evidence for chunk in chunks for evidence in chunk.catalog]
    assert "".join(item.exact_anchor for item in catalog) == script
    assert all(
        item.text_hash == hashlib.sha256(item.exact_anchor.encode()).hexdigest() for item in catalog
    )
    assert next(item for item in catalog if "Aurelia" in item.exact_anchor).episode_number == 1


@pytest.mark.asyncio
async def test_harness_checkpoints_each_phase_and_resumes_reviewed_result() -> None:
    script = "EPISODE 1\nINT. THRONE ROOM - DAY\nAurelia enters.\n"
    bible_input = _input(script)
    store = _MemoryCheckpointStore()
    evidence = _EvidenceModel()
    reconcile = _ReconcileModel()
    review = _ReviewModel()
    harness = ProductionBibleAgentHarness(
        models=ProductionBibleAgentModels(
            evidence=evidence,
            reconcile=reconcile,
            review=review,
        ),
        checkpoint_store=store,
    )

    result = await harness.run(bible_input)

    assert isinstance(result, ProductionBibleProviderResult)
    assert result.entities[0].entity_key == "character:aurelia"
    assert [checkpoint.stage for checkpoint in store.saved] == [
        "evidence",
        "reconciled",
        "reviewed",
    ]
    assert store.latest is not None
    assert store.latest.harness_version == PRODUCTION_BIBLE_HARNESS_VERSION

    resumed = await harness.run(replace(bible_input, run_token=uuid7()))

    assert resumed == result
    assert evidence.calls == 1
    assert reconcile.calls == 1
    assert review.calls == 1


@pytest.mark.asyncio
async def test_harness_resumes_after_the_last_completed_evidence_chunk() -> None:
    script = "EPISODE 1\n" + ("A" * 36_001)
    chunks = build_evidence_chunks(script)
    assert len(chunks) >= 3
    bible_input = _input(script)
    store = _MemoryCheckpointStore()

    class FailsSecondChunkOnce:
        def __init__(self) -> None:
            self.calls: list[str] = []
            self.failed = False

        async def ainvoke(self, messages: Sequence[Any]) -> object:
            payload = json.loads(messages[-1].content)
            chunk_key = payload["chunk_key"]
            self.calls.append(chunk_key)
            if chunk_key == chunks[1].key and not self.failed:
                self.failed = True
                raise RuntimeError("provider interrupted")
            return {
                "chunk_key": chunk_key,
                "source_start": payload["source_start"],
                "source_end": payload["source_end"],
                "observations": [],
            }

    class EmptyReconcileModel:
        async def ainvoke(self, messages: Sequence[Any]) -> object:
            payload = json.loads(messages[-1].content)
            assert "evidence_chunks" not in payload
            assert "evidence_catalog" in payload
            assert "observations" in payload
            return {"entities": [], "world_entries": [], "review_issues": []}

    evidence = FailsSecondChunkOnce()
    harness = ProductionBibleAgentHarness(
        models=ProductionBibleAgentModels(
            evidence=evidence,
            reconcile=EmptyReconcileModel(),
            review=_ReviewModel(),
        ),
        checkpoint_store=store,
    )

    with pytest.raises(RuntimeError, match="provider interrupted"):
        await harness.run(bible_input)

    assert store.latest is not None
    assert store.latest.completed_chunk_keys == (chunks[0].key,)

    result = await harness.run(replace(bible_input, run_token=uuid7()))

    assert result == ProductionBibleProviderResult()
    assert evidence.calls.count(chunks[0].key) == 1
    assert evidence.calls.count(chunks[1].key) == 2
    assert all(evidence.calls.count(chunk.key) == 1 for chunk in chunks[2:])


@pytest.mark.asyncio
async def test_reconcile_payload_deduplicates_evidence_and_preserves_all_references() -> None:
    script = "EPISODE 1\n" + ("A" * 126_000)
    chunks = build_evidence_chunks(script)
    assert len(chunks) == 8
    bible_input = _input(script)
    store = _MemoryCheckpointStore()

    class RepeatingEvidenceModel:
        async def ainvoke(self, messages: Sequence[Any]) -> object:
            payload = json.loads(messages[-1].content)
            evidence = payload["evidence_catalog"][0]
            return {
                "chunk_key": payload["chunk_key"],
                "source_start": payload["source_start"],
                "source_end": payload["source_end"],
                "observations": [
                    {
                        "observation_key": f"observation-{index + 1}",
                        "kind": "entity",
                        "subject_key": "repeated-subject",
                        "parent_entity_key": None,
                        "claim": f"Grounded claim {index + 1}.",
                        "evidence": [evidence],
                        "ambiguities": [],
                    }
                    for index in range(6)
                ],
            }

    class CapturingReconcileModel:
        def __init__(self) -> None:
            self.content: str | None = None

        async def ainvoke(self, messages: Sequence[Any]) -> object:
            self.content = messages[-1].content
            return {"entities": [], "world_entries": [], "review_issues": []}

    reconcile = CapturingReconcileModel()
    harness = ProductionBibleAgentHarness(
        models=ProductionBibleAgentModels(
            evidence=RepeatingEvidenceModel(),
            reconcile=reconcile,
            review=_ReviewModel(),
        ),
        checkpoint_store=store,
    )

    await harness.run(bible_input)

    assert reconcile.content is not None
    payload = json.loads(reconcile.content)
    assert "evidence_chunks" not in payload
    catalog = payload["evidence_catalog"]
    observations = payload["observations"]
    assert list(catalog) == [f"e{index:04d}" for index in range(1, 9)]
    assert len(observations) == 48
    assert all("evidence" not in observation for observation in observations)
    assert all(len(observation["evidence_refs"]) == 1 for observation in observations)
    assert {ref for observation in observations for ref in observation["evidence_refs"]} == set(
        catalog
    )

    assert store.latest is not None
    evidence_chunks = store.latest.evidence_chunks
    catalog_keys_by_evidence = {
        (
            evidence["source_start"],
            evidence["source_end"],
            evidence["text_hash"],
        ): evidence_key
        for evidence_key, evidence in catalog.items()
    }
    original_observations = [
        (chunk.chunk_key, observation)
        for chunk in evidence_chunks
        for observation in chunk.observations
    ]
    for compact, (chunk_key, original) in zip(
        observations,
        original_observations,
        strict=True,
    ):
        assert compact["chunk_key"] == chunk_key
        assert compact["observation_key"] == original.observation_key
        assert compact["evidence_refs"] == list(
            dict.fromkeys(
                catalog_keys_by_evidence[(item.source_start, item.source_end, item.text_hash)]
                for item in original.evidence
            )
        )

    legacy_content = json.dumps(
        {
            "document_revision_id": str(bible_input.document_revision_id),
            "input_hash": bible_input.input_hash,
            "evidence_chunks": [item.model_dump(mode="json") for item in evidence_chunks],
        },
        ensure_ascii=False,
        separators=(",", ":"),
    )
    assert len(reconcile.content.encode("utf-8")) * 2 < len(legacy_content.encode("utf-8"))

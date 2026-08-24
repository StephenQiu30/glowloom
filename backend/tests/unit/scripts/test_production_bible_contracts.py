from hashlib import sha256
from typing import cast

import pytest
from pydantic import ValidationError
from sqlalchemy import Table

from app.modules.scripts.production_bibles.models import ProductionBible
from app.modules.scripts.production_bibles.schemas import (
    BibleAssetSpecCandidate,
    BibleEntityCandidate,
    BibleEntityStateCandidate,
    BibleEvidence,
    BibleWorldEntryCandidate,
    ProductionBibleProviderResult,
    ProductionBibleResponse,
    ProductionBibleResumeRequest,
    ProductionBibleReviewIssueResolutionRequest,
)


def _evidence(text: str, *, start: int = 0, episode_number: int | None = 1) -> BibleEvidence:
    return BibleEvidence(
        source_start=start,
        source_end=start + len(text),
        text_hash=sha256(text.encode()).hexdigest(),
        exact_anchor=text,
        episode_number=episode_number,
    )


def test_provider_result_preserves_one_identity_with_episode_states() -> None:
    identity_evidence = _evidence("Aurelia wore a red cloak.")
    state_evidence = _evidence("The cloak was torn in battle.", start=30, episode_number=2)
    entity = BibleEntityCandidate(
        entity_key="character.aurelia",
        kind="character",
        canonical_name="Aurelia",
        normalized_name="aurelia",
        aliases=["The Empress"],
        stable_spec=BibleAssetSpecCandidate(identity="the reigning empress"),
        episode_numbers=[1, 2],
        evidence=[identity_evidence],
        states=[
            BibleEntityStateCandidate(
                state_key="torn_cloak",
                label="Battle-damaged cloak",
                state_spec=BibleAssetSpecCandidate(appearance="torn cloak"),
                episode_numbers=[2],
                evidence=[state_evidence],
            )
        ],
    )
    result = ProductionBibleProviderResult(
        entities=[entity],
        world_entries=[
            BibleWorldEntryCandidate(
                entry_key="rule.imperial_succession",
                category="politics",
                title="Imperial succession",
                facts=["Aurelia is the reigning empress."],
                rules=["The imperial seal determines lawful succession."],
                entity_keys=["character.aurelia"],
                episode_numbers=[1, 2],
                evidence=[identity_evidence],
            )
        ],
    )

    assert len(result.entities) == 1
    assert result.entities[0].states[0].episode_numbers == [2]
    assert result.world_entries[0].entity_keys == ["character.aurelia"]


def test_evidence_rejects_non_exact_anchor_hash() -> None:
    with pytest.raises(ValidationError, match="text_hash must match exact_anchor"):
        BibleEvidence(
            source_start=10,
            source_end=15,
            text_hash="0" * 64,
            exact_anchor="queen",
            episode_number=None,
        )


def test_provider_result_rejects_dangling_world_entity_reference() -> None:
    evidence = _evidence("The gate opens only for the royal seal.")

    with pytest.raises(ValidationError, match="unknown entity keys"):
        ProductionBibleProviderResult(
            world_entries=[
                BibleWorldEntryCandidate(
                    entry_key="rule.royal_gate",
                    category="magic",
                    title="Royal gate",
                    rules=["The gate opens only for the royal seal."],
                    entity_keys=["prop.royal_seal"],
                    episode_numbers=[1],
                    evidence=[evidence],
                )
            ]
        )


def test_strict_contract_rejects_extra_fields() -> None:
    text = "Aurelia"

    with pytest.raises(ValidationError, match="Extra inputs are not permitted"):
        BibleEvidence.model_validate(
            {
                "source_start": 0,
                "source_end": len(text),
                "text_hash": sha256(text.encode()).hexdigest(),
                "exact_anchor": text,
                "episode_number": 1,
                "unexpected": True,
            }
        )


def test_persistence_has_paired_private_lease_and_response_hides_it() -> None:
    table = cast(Table, ProductionBible.__table__)
    constraint_names = {constraint.name for constraint in table.constraints}

    assert "run_token" in table.c
    assert "lease_expires_at" in table.c
    assert "resume_receipts" in table.c
    assert "review_receipts" in table.c
    assert "ck_scr_prod_bible_lease" in constraint_names
    assert "ck_scr_prod_bible_resume_receipts" in constraint_names
    assert "ck_scr_prod_bible_review_receipts" in constraint_names
    assert "checkpoint_stage" in ProductionBibleResponse.model_fields
    assert "checkpoint_revision" in ProductionBibleResponse.model_fields
    assert "run_token" not in ProductionBibleResponse.model_fields
    assert "lease_expires_at" not in ProductionBibleResponse.model_fields


def test_resume_request_is_strict_and_immutable() -> None:
    request = ProductionBibleResumeRequest(
        expected_revision=3,
        idempotency_key="resume-001",
    )

    with pytest.raises(ValidationError, match="Instance is frozen"):
        request.expected_revision = 4
    with pytest.raises(ValidationError, match="Extra inputs are not permitted"):
        ProductionBibleResumeRequest.model_validate(
            {
                "expected_revision": 3,
                "idempotency_key": "resume-001",
                "timeout": 30,
            }
        )


def test_review_issue_resolution_requires_sorted_unique_episode_numbers() -> None:
    request = ProductionBibleReviewIssueResolutionRequest.model_validate(
        {
            "expected_revision": 6,
            "expected_result_hash": "a" * 64,
            "idempotency_key": "resolve-signed-state-001",
            "issue_key": "issue:eos_assignment_signed_episode_mapping",
            "resolution_note": "Episode 32 is conditional; the signature happens in episode 33.",
            "correction": {
                "kind": "entity_state_episode_numbers",
                "entity_key": "prop:eos_9_collateral_assignment",
                "state_key": "signed",
                "episode_numbers": [33],
            },
        }
    )

    assert request.correction.episode_numbers == [33]
    with pytest.raises(ValidationError, match="sorted unique"):
        ProductionBibleReviewIssueResolutionRequest.model_validate(
            {
                **request.model_dump(mode="json"),
                "correction": {
                    **request.correction.model_dump(mode="json"),
                    "episode_numbers": [33, 32, 33],
                },
            }
        )

import json
import re
from pathlib import Path
from typing import Literal

import pytest
from pydantic import BaseModel, ConfigDict, Field, TypeAdapter

FIXTURE_DIRECTORY = Path(__file__).parents[1] / "fixtures/mvp_a"
GOLDEN_CANDIDATE_FILE = FIXTURE_DIRECTORY / "golden_candidate_harbor_countdown.json"
DOCS_IMPORT_SAMPLE_FILE = (
    Path(__file__).parents[3]
    / "docs/fixtures/mvp_a/002-雾港倒计时整剧导入样例.txt"
)


class StrictFixtureModel(BaseModel):
    model_config = ConfigDict(extra="forbid")


class ProvenanceFixture(StrictFixtureModel):
    content_origin: Literal["original_synthetic"]
    structural_reference: Literal["user_supplied_english_docx"]
    structural_reference_features: list[
        Literal[
            "explicit_episode_marker",
            "scene_heading_action_dialogue",
            "compact_episode_hook",
        ]
    ]
    copied_or_translated_source_text: Literal[False]
    repository_contains_source_docx: Literal[False]


class AuthorizationFixture(StrictFixtureModel):
    authorized_for_repository: Literal[True]
    authorized_on: Literal["2026-08-13"]
    authorized_scope: Literal["MVP-A工程材料与自动化契约"]
    product_owner_material_approval: Literal[True]
    short_drama_producer_quality_signoff: bool
    qa_oracle_signoff: bool


class EpisodeFixture(StrictFixtureModel):
    episode_id: str = Field(pattern=r"^ep-0[1-5]$")
    episode_number: int = Field(ge=1, le=5)
    title: str = Field(min_length=1)
    target_duration_seconds: int = Field(ge=60, le=120)
    source_start_codepoint: int = Field(ge=0)
    source_end_codepoint: int = Field(gt=0)
    source_text: str = Field(min_length=1)


class NarrativeUnitFixture(StrictFixtureModel):
    narrative_unit_id: str = Field(pattern=r"^nu-03-\d{3}$")
    episode_id: Literal["ep-03"]
    order: int = Field(ge=1)
    kind: Literal["scene_heading", "action", "dialogue", "narration"]
    source_start_codepoint: int = Field(ge=0)
    source_end_codepoint: int = Field(gt=0)
    exact_text: str = Field(min_length=1)
    required_for_coverage: bool
    coverage_reason: str = Field(min_length=1)


class AssetStateFixture(StrictFixtureModel):
    state_id: str = Field(pattern=r"^state-[a-z0-9-]+$")
    state_key: str = Field(pattern=r"^[a-z0-9_]+$")
    label: str = Field(min_length=1)
    asset_version_id: str = Field(pattern=r"^av-[a-z0-9-]+-001$")
    episode_ids: list[str] = Field(min_length=1)
    narrative_unit_ids: list[str]


class AssetFixture(StrictFixtureModel):
    asset_id: str = Field(pattern=r"^asset-[a-z0-9-]+$")
    kind: Literal["character", "location", "prop"]
    name: str = Field(min_length=1)
    states: list[AssetStateFixture] = Field(min_length=1)


class ShotAssetRefFixture(StrictFixtureModel):
    index: int = Field(ge=1)
    asset_state_id: str
    asset_version_id: str
    display_name: str = Field(min_length=1)


class ShotFixture(StrictFixtureModel):
    shot_id: str = Field(pattern=r"^shot-03-\d{3}$")
    order: int = Field(ge=1)
    duration_seconds: int = Field(ge=4, le=15)
    framing: str = Field(min_length=1)
    camera: str = Field(min_length=1)
    description: str = Field(min_length=1)
    prompt: str = Field(min_length=1)
    narrative_unit_ids: list[str] = Field(min_length=1)
    asset_refs: list[ShotAssetRefFixture] = Field(min_length=1)
    asset_ref_source: Literal["oracle_manual"]
    creative_waiver_id: str | None


class OmissionFixture(StrictFixtureModel):
    narrative_unit_id: str
    decision: Literal["omit"]
    reason: str = Field(min_length=1)


class CreativeWaiverFixture(StrictFixtureModel):
    creative_waiver_id: str
    shot_id: str
    decision: Literal["approved"]
    reason: str = Field(min_length=1)


class CoverageMappingFixture(StrictFixtureModel):
    narrative_unit_ids: list[str] = Field(min_length=1)
    shot_ids: list[str] = Field(min_length=1)


class ConservationMappingFixture(CoverageMappingFixture):
    invariant: str = Field(min_length=1)


class MappingExamplesFixture(StrictFixtureModel):
    one_narrative_unit_to_many_shots: CoverageMappingFixture
    many_narrative_units_to_one_shot: CoverageMappingFixture
    split_coverage_conservation: ConservationMappingFixture
    merge_coverage_conservation: ConservationMappingFixture


class StoryboardOracleFixture(StrictFixtureModel):
    episode_id: Literal["ep-03"]
    expected_shot_count: int = Field(ge=12, le=24)
    expected_total_duration_seconds: int = Field(ge=60, le=120)
    shots: list[ShotFixture] = Field(min_length=12, max_length=24)
    approved_omissions: list[OmissionFixture]
    creative_waivers: list[CreativeWaiverFixture]
    mapping_examples: MappingExamplesFixture


class ReviewStatusFixture(StrictFixtureModel):
    engineering_contract: Literal["accepted"]
    content_quality_gate: Literal["awaiting_short_drama_producer_and_qa"]
    closes_g_mvpa_002: Literal[True]


class GoldenCandidateFixture(StrictFixtureModel):
    fixture_id: Literal["mvp-a-golden-candidate-harbor-countdown-001"]
    classification: Literal["synthetic_engineering_golden_candidate"]
    title: Literal["雾港倒计时"]
    language: Literal["zh-CN"]
    aspect_ratio: Literal["9:16"]
    provenance: ProvenanceFixture
    authorization: AuthorizationFixture
    full_script: str = Field(min_length=1, max_length=100_000)
    episodes: list[EpisodeFixture] = Field(min_length=5, max_length=5)
    selected_episode_id: Literal["ep-03"]
    narrative_units: list[NarrativeUnitFixture] = Field(min_length=1)
    assets: list[AssetFixture] = Field(min_length=1)
    storyboard_oracle: StoryboardOracleFixture
    review_status: ReviewStatusFixture


def load_golden_candidate() -> GoldenCandidateFixture:
    raw: object = json.loads(GOLDEN_CANDIDATE_FILE.read_text(encoding="utf-8"))
    return TypeAdapter(GoldenCandidateFixture).validate_python(raw)


def test_golden_candidate_is_original_authorized_material_without_source_copy() -> None:
    fixture = load_golden_candidate()
    serialized = GOLDEN_CANDIDATE_FILE.read_text(encoding="utf-8")

    assert fixture.provenance.structural_reference_features == [
        "explicit_episode_marker",
        "scene_heading_action_dialogue",
        "compact_episode_hook",
    ]
    assert fixture.authorization.authorized_for_repository is True
    assert fixture.authorization.product_owner_material_approval is True
    assert fixture.provenance.copied_or_translated_source_text is False
    assert fixture.provenance.repository_contains_source_docx is False
    assert "/Users/" not in serialized
    assert "http://" not in serialized
    assert "https://" not in serialized


def test_docs_import_sample_is_utf8_without_bom_and_matches_golden_script() -> None:
    fixture = load_golden_candidate()
    raw_bytes = DOCS_IMPORT_SAMPLE_FILE.read_bytes()

    assert not raw_bytes.startswith(b"\xef\xbb\xbf")
    raw_text = raw_bytes.decode("utf-8")
    assert raw_text.endswith("\n")
    assert not raw_text.endswith("\n\n")
    assert raw_text.removesuffix("\n") == fixture.full_script


def test_five_episode_boundaries_are_exact_contiguous_codepoint_slices() -> None:
    fixture = load_golden_candidate()

    assert [episode.episode_number for episode in fixture.episodes] == [1, 2, 3, 4, 5]
    assert [episode.episode_id for episode in fixture.episodes] == [
        "ep-01",
        "ep-02",
        "ep-03",
        "ep-04",
        "ep-05",
    ]
    assert fixture.full_script == "\n\n".join(
        episode.source_text for episode in fixture.episodes
    )

    previous_end = 0
    expected_markers = ["第一集", "第二集", "第三集", "第四集", "第五集"]
    for episode, marker in zip(fixture.episodes, expected_markers, strict=True):
        assert episode.source_start_codepoint == previous_end
        assert fixture.full_script[
            episode.source_start_codepoint : episode.source_end_codepoint
        ] == episode.source_text
        assert episode.source_text.splitlines()[0] == marker
        previous_end = episode.source_end_codepoint + 2

    assert fixture.episodes[-1].source_end_codepoint == len(fixture.full_script)


def test_selected_episode_narrative_units_preserve_exact_source_ranges() -> None:
    fixture = load_golden_candidate()
    selected = next(
        episode for episode in fixture.episodes if episode.episode_id == fixture.selected_episode_id
    )

    assert [unit.order for unit in fixture.narrative_units] == list(range(1, 21))
    assert {unit.kind for unit in fixture.narrative_units} == {
        "scene_heading",
        "action",
        "dialogue",
        "narration",
    }
    previous_end = 0
    for unit in fixture.narrative_units:
        assert unit.source_start_codepoint >= previous_end
        assert selected.source_text[
            unit.source_start_codepoint : unit.source_end_codepoint
        ] == unit.exact_text
        previous_end = unit.source_end_codepoint


def test_storyboard_oracle_has_sixteen_shots_and_ninety_two_seconds() -> None:
    fixture = load_golden_candidate()
    oracle = fixture.storyboard_oracle

    assert fixture.aspect_ratio == "9:16"
    assert oracle.expected_shot_count == 16
    assert len(oracle.shots) == oracle.expected_shot_count
    assert [shot.order for shot in oracle.shots] == list(range(1, 17))
    assert sum(shot.duration_seconds for shot in oracle.shots) == 92
    assert oracle.expected_total_duration_seconds == 92


def test_required_units_are_covered_and_optional_unit_has_an_approved_omission() -> None:
    fixture = load_golden_candidate()
    oracle = fixture.storyboard_oracle
    covered = {
        unit_id for shot in oracle.shots for unit_id in shot.narrative_unit_ids
    }
    required = {
        unit.narrative_unit_id
        for unit in fixture.narrative_units
        if unit.required_for_coverage
    }
    optional = {
        unit.narrative_unit_id
        for unit in fixture.narrative_units
        if not unit.required_for_coverage
    }
    omitted = {decision.narrative_unit_id for decision in oracle.approved_omissions}

    assert covered == required
    assert optional == {"nu-03-018"}
    assert omitted == optional
    assert covered.isdisjoint(omitted)


def test_shot_and_narrative_mapping_is_explicitly_many_to_many() -> None:
    fixture = load_golden_candidate()
    oracle = fixture.storyboard_oracle
    by_id = {shot.shot_id: shot for shot in oracle.shots}
    examples = oracle.mapping_examples

    one_to_many = examples.one_narrative_unit_to_many_shots
    assert one_to_many.narrative_unit_ids == ["nu-03-002"]
    assert all(
        "nu-03-002" in by_id[shot_id].narrative_unit_ids
        for shot_id in one_to_many.shot_ids
    )
    assert len(one_to_many.shot_ids) > 1

    many_to_one = examples.many_narrative_units_to_one_shot
    assert many_to_one.shot_ids == ["shot-03-003"]
    assert set(many_to_one.narrative_unit_ids) <= set(
        by_id["shot-03-003"].narrative_unit_ids
    )
    assert len(many_to_one.narrative_unit_ids) > 1


def test_split_and_merge_oracles_conserve_narrative_references() -> None:
    fixture = load_golden_candidate()
    by_id = {shot.shot_id: shot for shot in fixture.storyboard_oracle.shots}
    examples = fixture.storyboard_oracle.mapping_examples

    split = examples.split_coverage_conservation
    split_union = {
        unit_id for shot_id in split.shot_ids for unit_id in by_id[shot_id].narrative_unit_ids
    }
    assert set(split.narrative_unit_ids) <= split_union
    assert len(split.shot_ids) == 3

    merge = examples.merge_coverage_conservation
    merged_refs = set(by_id[merge.shot_ids[0]].narrative_unit_ids)
    assert merged_refs == set(merge.narrative_unit_ids)


def test_asset_states_cover_character_location_and_prop_transitions() -> None:
    fixture = load_golden_candidate()
    states_by_asset = {
        asset.name: {state.state_key for state in asset.states} for asset in fixture.assets
    }

    assert {"normal", "injured"} <= states_by_asset["沈岚"]
    assert {"idle_evening", "flooded_night"} <= states_by_asset["雾港旧泵站"]
    assert {"intact", "broken"} <= states_by_asset["铜制检修钥匙"]
    assert {"intact", "water_damaged", "restored"} <= states_by_asset["黑色录音笔"]

    episode_ids = {episode.episode_id for episode in fixture.episodes}
    narrative_unit_ids = {unit.narrative_unit_id for unit in fixture.narrative_units}
    state_ids: set[str] = set()
    version_ids: set[str] = set()
    for asset in fixture.assets:
        for state in asset.states:
            assert state.state_id not in state_ids
            assert state.asset_version_id not in version_ids
            assert set(state.episode_ids) <= episode_ids
            assert set(state.narrative_unit_ids) <= narrative_unit_ids
            state_ids.add(state.state_id)
            version_ids.add(state.asset_version_id)


def test_every_prompt_token_has_one_ordered_fixed_asset_version_reference() -> None:
    fixture = load_golden_candidate()
    states = {
        state.state_id: state for asset in fixture.assets for state in asset.states
    }

    for shot in fixture.storyboard_oracle.shots:
        assert [ref.index for ref in shot.asset_refs] == list(
            range(1, len(shot.asset_refs) + 1)
        )
        assert [int(index) for index in re.findall(r"@图片(\d+)", shot.prompt)] == [
            ref.index for ref in shot.asset_refs
        ]
        for ref in shot.asset_refs:
            assert ref.asset_state_id in states
            assert ref.asset_version_id == states[ref.asset_state_id].asset_version_id


def test_creative_shot_requires_a_matching_waiver_and_gate_separates_quality_review() -> None:
    fixture = load_golden_candidate()
    oracle = fixture.storyboard_oracle
    waivers = {waiver.creative_waiver_id: waiver for waiver in oracle.creative_waivers}

    creative_shots = [shot for shot in oracle.shots if shot.creative_waiver_id]
    assert [shot.shot_id for shot in creative_shots] == ["shot-03-012"]
    for shot in creative_shots:
        assert shot.creative_waiver_id is not None
        waiver = waivers[shot.creative_waiver_id]
        assert waiver.shot_id == shot.shot_id

    assert fixture.authorization.short_drama_producer_quality_signoff is False
    assert fixture.authorization.qa_oracle_signoff is False
    assert fixture.review_status.engineering_contract == "accepted"
    assert fixture.review_status.closes_g_mvpa_002 is True


def test_contract_rejects_fixture_without_product_owner_repository_authorization() -> None:
    raw = json.loads(GOLDEN_CANDIDATE_FILE.read_text(encoding="utf-8"))
    raw["authorization"]["product_owner_material_approval"] = False

    with pytest.raises(ValueError, match="Input should be True"):
        GoldenCandidateFixture.model_validate(raw)

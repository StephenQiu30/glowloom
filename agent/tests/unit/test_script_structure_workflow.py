import json
from collections.abc import Sequence
from typing import Any, cast

import pytest
from langchain_core.messages import HumanMessage
from uuid6 import uuid7

from app.modules.scripts.contracts import (
    ProductionBibleEntityInput,
    ProductionBibleExtractionInput,
    ProductionBibleStateInput,
    ProductionBibleWorldInput,
)
from app.modules.scripts.extractions.anchoring import anchor_script_structure_ranges
from app.modules.scripts.extractions.schemas import (
    AssetCandidateProposal,
    AssetOccurrenceCandidateProposal,
    ContinuityCandidateProposal,
    DialogueCandidateProposal,
    SceneCandidateProposal,
    ShotCandidateProposal,
)
from app.modules.skills.contracts import SkillDefinition, SkillExecutionContext, SkillExecutionError
from app.modules.skills.script_structure import (
    ScriptExtractionChunk,
    ScriptStructureExtractionWorkflow,
    segment_script,
)


def _skill() -> SkillDefinition:
    return SkillDefinition(
        name="script.structure.extract",
        version="v2",
        max_input_chars=20_000,
    )


def _context() -> SkillExecutionContext:
    return SkillExecutionContext(
        skill_name="script.structure.extract",
        skill_version="v2",
        workspace_id="workspace-test",
        task_id="task-test",
    )


def _production_bible() -> ProductionBibleExtractionInput:
    entity_key = "character:mara"
    return ProductionBibleExtractionInput(
        bible_id=uuid7(),
        revision=4,
        result_hash="a" * 64,
        entities=(
            ProductionBibleEntityInput(
                entity_key=entity_key,
                kind="character",
                canonical_name="Mara",
                aliases=("The Empress",),
                stable_spec={"identity": "The same protagonist across every episode"},
                states=(
                    ProductionBibleStateInput(
                        entity_key=entity_key,
                        state_key="base",
                        label="Base",
                        asset_state_id=uuid7(),
                        asset_version_id=uuid7(),
                        state_spec={"appearance": "Black hair and an imperial signet"},
                    ),
                ),
            ),
            ProductionBibleEntityInput(
                entity_key="location:palace",
                kind="location",
                canonical_name="Imperial Palace",
                aliases=("PALACE",),
                stable_spec={"identity": "The imperial residence"},
                states=(
                    ProductionBibleStateInput(
                        entity_key="location:palace",
                        state_key="base",
                        label="Base",
                        asset_state_id=uuid7(),
                        asset_version_id=uuid7(),
                        state_spec={},
                    ),
                    ProductionBibleStateInput(
                        entity_key="location:palace",
                        state_key="under_siege",
                        label="Under Siege",
                        asset_state_id=uuid7(),
                        asset_version_id=uuid7(),
                        state_spec={"condition": "The palace is under siege"},
                    ),
                ),
            ),
            ProductionBibleEntityInput(
                entity_key="prop:water_panel",
                kind="prop",
                canonical_name="Water Recycling Control Panel",
                aliases=(),
                stable_spec={"identity": "The estate water-system control panel"},
                states=(
                    ProductionBibleStateInput(
                        entity_key="prop:water_panel",
                        state_key="base",
                        label="Base",
                        asset_state_id=uuid7(),
                        asset_version_id=uuid7(),
                        state_spec={},
                    ),
                ),
            ),
        ),
        world_entries=(
            ProductionBibleWorldInput(
                entry_key="world:imperial_identity",
                category="identity_rule",
                title="Hidden imperial identity",
                facts=("Mara is the Empress.",),
                rules=("Her signet remains visually consistent.",),
                entity_keys=(entity_key,),
            ),
        ),
    )


def test_segment_script_preserves_episode_and_scene_boundaries() -> None:
    script = "\n".join(
        [
            "Title",
            "EPISODE 1",
            "INT. HOUSE - DAY",
            "A" * 1_200,
            "EXT. ROAD - NIGHT",
            "B" * 1_200,
            "EPISODE 2",
            "INT. OFFICE - DAY",
            "C" * 1_200,
        ]
    )

    chunks = segment_script(script, max_chunk_chars=1_500)

    assert "".join(chunk.text for chunk in chunks) == script
    assert {chunk.episode_number for chunk in chunks} == {None, 1, 2}
    assert any(chunk.episode_number == 1 and "EXT. ROAD - NIGHT" in chunk.text for chunk in chunks)
    assert all(len(chunk.text) <= 1_500 for chunk in chunks)


def test_segment_script_maps_each_chinese_scene_independently() -> None:
    script = "\n".join(
        [
            "# 《雾港来信》",
            "外景·旧车站·暴雨夜",
            "林澈收到旧信。",
            "内景·候车厅·连续",
            "红雨衣女人出现。",
            "内景·行李房·夜",
            "林澈发现录音机。",
        ]
    )

    chunks = segment_script(script, max_chunk_chars=1_000)

    assert len(chunks) == 3
    assert "外景·旧车站·暴雨夜" in chunks[0].text
    assert "内景·候车厅·连续" in chunks[1].text
    assert "内景·行李房·夜" in chunks[2].text
    assert all(
        sum(
            heading in chunk.text
            for heading in (
                "外景·旧车站·暴雨夜",
                "内景·候车厅·连续",
                "内景·行李房·夜",
            )
        )
        == 1
        for chunk in chunks
    )


@pytest.mark.asyncio
async def test_workflow_applies_explicit_episode_number_without_source_marker() -> None:
    model = _FakeStructureModel()
    workflow = ScriptStructureExtractionWorkflow(
        skill=_skill(),
        model=model,
        system_prompt="Extract screenplay structure.",
    )

    result = await workflow.run(
        "INT. HOUSE - DAY\nMara looks outside.",
        context=_context(),
        episode_number=2,
    )

    assert model.payloads[0]["episode_number"] == 2
    scene = next(candidate for candidate in result.candidates if candidate.proposal.kind == "scene")
    character = next(
        candidate for candidate in result.candidates if candidate.proposal.kind == "asset"
    )
    assert isinstance(scene.proposal, SceneCandidateProposal)
    assert isinstance(character.proposal, AssetCandidateProposal)
    assert scene.proposal.episode_number == 2
    assert character.proposal.first_seen_episode == 2
    assert character.proposal.episode_numbers == [2]


@pytest.mark.asyncio
async def test_workflow_completes_scene_skeleton_when_model_omits_supported_heading() -> None:
    script = (
        "EPISODE 1\n"
        "INT. HOUSE - DAY\nCARD: HOURS AGO\nMara checks the door.\n"
        "I/E. HOUSE / WATER SHAFT - NIGHT\nThe children hear the storm."
    )

    class OmittedSceneModel:
        async def ainvoke(self, messages: Sequence[Any]) -> object:
            message = cast(HumanMessage, messages[1])
            payload = cast(dict[str, Any], json.loads(cast(str, message.content)))
            chunk = cast(str, payload["script_text"])
            candidates: list[dict[str, object]] = [
                {
                    "candidate_key": "episode-1",
                    "source_range": {"start": 0, "end": len(chunk)},
                    "proposal": {
                        "kind": "continuity",
                        "scope": "episode",
                        "severity": "info",
                        "issue": "The storm isolates the children.",
                        "suggestion": "Preserve the storm escalation.",
                        "episode_number": 1,
                        "title": "The Storm",
                        "logline": "Mara leaves before the storm reaches the children.",
                        "summary": "The household separates as the storm arrives.",
                    },
                }
            ]
            if "INT. HOUSE - DAY" in chunk:
                start = chunk.index("INT. HOUSE - DAY")
                card_start = chunk.index("CARD: HOURS AGO")
                candidates[0:0] = [
                    {
                        "candidate_key": "scene-house",
                        "source_range": {"start": start, "end": len(chunk)},
                        "proposal": {
                            "kind": "scene",
                            "heading": "INT. HOUSE - DAY",
                            "location": "HOUSE",
                            "time_of_day": "DAY",
                            "summary": "Mara checks the door.",
                            "episode_number": 1,
                        },
                    },
                    {
                        "candidate_key": "hallucinated-card-scene",
                        "source_range": {"start": card_start, "end": len(chunk)},
                        "proposal": {
                            "kind": "scene",
                            "heading": "CARD: HOURS AGO",
                            "location": "HOUSE",
                            "time_of_day": "DAY",
                            "summary": "A time card appears.",
                            "episode_number": 1,
                        },
                    },
                    {
                        "candidate_key": "card-dialogue",
                        "source_range": {"start": card_start, "end": len(chunk)},
                        "proposal": {
                            "kind": "dialogue",
                            "scene_candidate_key": "hallucinated-card-scene",
                            "speaker_candidate": "Mara",
                            "dialogue_kind": "spoken",
                            "text": "Check the door.",
                        },
                    },
                ]
            return {"candidates": candidates}

    workflow = ScriptStructureExtractionWorkflow(
        skill=_skill(),
        model=OmittedSceneModel(),
        system_prompt="Extract screenplay structure.",
    )

    result = await workflow.run(
        script,
        context=_context(),
        episode_number=1,
    )

    scenes = [
        candidate
        for candidate in result.candidates
        if isinstance(candidate.proposal, SceneCandidateProposal)
    ]
    assert [cast(SceneCandidateProposal, scene.proposal).heading for scene in scenes] == [
        "INT. HOUSE - DAY",
        "I/E. HOUSE / WATER SHAFT - NIGHT",
    ]
    fallback = scenes[1]
    fallback_proposal = cast(SceneCandidateProposal, fallback.proposal)
    assert fallback.source_range.start == script.index("I/E. HOUSE / WATER SHAFT - NIGHT")
    assert fallback_proposal.location == "HOUSE / WATER SHAFT"
    assert fallback_proposal.time_of_day == "NIGHT"
    assert fallback.confidence_note == (
        "Deterministic scene heading fallback; semantic enrichment requires review."
    )
    dialogue = next(
        candidate
        for candidate in result.candidates
        if candidate.candidate_key.endswith(":card-dialogue")
    )
    assert isinstance(dialogue.proposal, DialogueCandidateProposal)
    assert dialogue.proposal.scene_candidate_key.endswith(":scene-house")
    anchored = anchor_script_structure_ranges(result, script)
    assert [
        candidate.proposal.heading
        for candidate in anchored.candidates
        if isinstance(candidate.proposal, SceneCandidateProposal)
    ] == ["INT. HOUSE - DAY", "I/E. HOUSE / WATER SHAFT - NIGHT"]


@pytest.mark.asyncio
async def test_bible_bound_workflow_passes_fixed_context_and_emits_occurrences() -> None:
    class BibleBoundModel:
        def __init__(self) -> None:
            self.payload: dict[str, Any] = {}

        async def ainvoke(self, messages: Sequence[Any]) -> object:
            message = cast(HumanMessage, messages[1])
            self.payload = cast(dict[str, Any], json.loads(cast(str, message.content)))
            return {
                "candidates": [
                    {
                        "candidate_key": "scene-1",
                        "source_range": {"start": 0, "end": 16},
                        "proposal": {
                            "kind": "scene",
                            "heading": "INT. PALACE - DAY",
                            "location": "PALACE",
                            "time_of_day": "DAY",
                            "summary": "Mara enters.",
                            "episode_number": 2,
                        },
                    },
                    {
                        "candidate_key": "mara-occurrence",
                        "source_range": {"start": 0, "end": 16},
                        "proposal": {
                            "kind": "asset_occurrence",
                            "entity_key": "character:mara",
                            "state_key": "base",
                            "scene_candidate_key": "scene-1",
                            "role": "character",
                        },
                    },
                    {
                        "candidate_key": "episode-2",
                        "source_range": {"start": 0, "end": 16},
                        "proposal": {
                            "kind": "continuity",
                            "scope": "episode",
                            "severity": "info",
                            "issue": "Mara enters the palace.",
                            "suggestion": "Keep the signet consistent.",
                            "episode_number": 2,
                        },
                    },
                ]
            }

    model = BibleBoundModel()
    workflow = ScriptStructureExtractionWorkflow(
        skill=_skill(),
        model=model,
        system_prompt="Extract screenplay structure.",
    )
    bible = _production_bible()

    result = await workflow.run(
        "INT. PALACE - DAY\nMara checks the WATER-RECYCLING CONTROL PANEL.",
        context=_context(),
        episode_number=2,
        production_bible=bible,
    )

    assert model.payload["asset_policy"].startswith("Reference only")
    assert model.payload["production_bible"]["bible_id"] == str(bible.bible_id)
    occurrences = [
        candidate.proposal
        for candidate in result.candidates
        if isinstance(candidate.proposal, AssetOccurrenceCandidateProposal)
    ]
    assert {occurrence.entity_key for occurrence in occurrences} == {
        "character:mara",
        "location:palace",
        "prop:water_panel",
    }
    assert {
        occurrence.state_key
        for occurrence in occurrences
        if occurrence.entity_key == "location:palace"
    } == {"base", "under_siege"}
    assert all(
        occurrence.scene_candidate_key == "scene-1"
        for occurrence in occurrences
        if occurrence.entity_key == "location:palace"
    )
    assert all(candidate.proposal.kind != "asset" for candidate in result.candidates)


@pytest.mark.asyncio
async def test_bible_bound_workflow_rejects_independent_asset_candidates() -> None:
    model = _FakeStructureModel()
    workflow = ScriptStructureExtractionWorkflow(
        skill=_skill(),
        model=model,
        system_prompt="Extract screenplay structure.",
    )

    with pytest.raises(SkillExecutionError, match="independent asset"):
        await workflow.run(
            "INT. HOUSE - DAY\nMara looks outside.",
            context=_context(),
            episode_number=2,
            production_bible=_production_bible(),
        )


@pytest.mark.asyncio
async def test_workflow_accepts_authoritative_marker_for_later_episode() -> None:
    model = _FakeStructureModel()
    workflow = ScriptStructureExtractionWorkflow(
        skill=_skill(),
        model=model,
        system_prompt="Extract screenplay structure.",
    )

    result = await workflow.run(
        "第二集\nINT. HOUSE - DAY\nMara looks outside.",
        context=_context(),
        episode_number=2,
    )

    assert model.payloads[0]["episode_number"] == 2
    assert result.candidates


@pytest.mark.asyncio
async def test_workflow_bounds_probabilistic_candidate_ranges_to_chunk() -> None:
    script = "外景·旧车站·夜\n林澈等待。"

    class OutOfBoundsModel:
        async def ainvoke(self, messages: Sequence[Any]) -> object:
            del messages
            return {
                "candidates": [
                    {
                        "candidate_key": "scene-1",
                        "source_range": {"start": 4, "end": 9_999},
                        "proposal": {
                            "kind": "scene",
                            "heading": "外景·旧车站·夜",
                            "location": "旧车站",
                            "time_of_day": "夜",
                            "summary": "林澈等待。",
                        },
                    }
                ]
            }

    workflow = ScriptStructureExtractionWorkflow(
        skill=_skill(),
        model=OutOfBoundsModel(),
        system_prompt="Extract screenplay structure.",
    )

    result = await workflow.run(script, context=_context())

    assert result.candidates[0].source_range.start == 4
    assert result.candidates[0].source_range.end == len(script)


class _FakeStructureModel:
    def __init__(self) -> None:
        self.payloads: list[dict[str, Any]] = []

    async def ainvoke(self, messages: Sequence[Any]) -> object:
        message = cast(HumanMessage, messages[1])
        payload = json.loads(cast(str, message.content))
        self.payloads.append(payload)
        episode_number = payload["episode_number"]
        script_text = payload["script_text"]
        end = min(24, len(script_text))
        scene_key = "scene-001"
        return {
            "candidates": [
                {
                    "candidate_key": scene_key,
                    "source_range": {"start": 0, "end": end},
                    "proposal": {
                        "kind": "scene",
                        "heading": "INT. HOUSE - DAY",
                        "location": "HOUSE",
                        "time_of_day": "DAY",
                        "summary": "主角在屋内确认危机。",
                        "episode_number": episode_number,
                        "scene_number": 1,
                        "story_beat": "危机建立",
                        "characters": ["Mara"],
                        "production_tasks": [
                            {
                                "task_type": "shot_breakdown",
                                "title": "拆解危机镜头",
                                "objective": "把场景动作拆为可执行镜头。",
                                "priority": "high",
                            }
                        ],
                    },
                },
                {
                    "candidate_key": "dialogue-001",
                    "source_range": {"start": 0, "end": end},
                    "proposal": {
                        "kind": "dialogue",
                        "scene_candidate_key": scene_key,
                        "speaker_candidate": "Mara",
                        "dialogue_kind": "spoken",
                        "text": "We need to move.",
                        "emotion": "urgent",
                    },
                },
                {
                    "candidate_key": "shot-001",
                    "source_range": {"start": 0, "end": end},
                    "proposal": {
                        "kind": "shot",
                        "scene_candidate_key": scene_key,
                        "title": "确认危机",
                        "purpose": "建立紧迫感",
                        "shot_type": "close_up",
                        "asset_names": ["Mara"],
                    },
                },
                {
                    "candidate_key": "asset-mara",
                    "source_range": {"start": 0, "end": end},
                    "proposal": {
                        "kind": "asset",
                        "asset_kind": "character",
                        "name": "Mara",
                        "description": "危机中的母亲。",
                        "role": "protagonist",
                        "first_seen_episode": episode_number,
                    },
                },
                {
                    "candidate_key": "episode-001",
                    "source_range": {"start": 0, "end": end},
                    "proposal": {
                        "kind": "continuity",
                        "scope": "episode",
                        "severity": "info",
                        "issue": "第 1 集结构摘要",
                        "suggestion": "保留本集的危机推进。",
                        "episode_number": episode_number,
                        "title": "危机建立",
                        "logline": "主角确认危机并开始行动。",
                        "summary": "主角在屋内确认危机，准备离开。",
                    },
                },
            ]
        }


@pytest.mark.asyncio
async def test_workflow_maps_ranges_references_and_deduplicates_assets() -> None:
    script = "EPISODE 1\nINT. HOUSE - DAY\n" + "Mara looks outside.\n" * 20
    midpoint = len(script) // 2
    model = _FakeStructureModel()

    def chunker(value: str) -> tuple[ScriptExtractionChunk, ...]:
        return (
            ScriptExtractionChunk("ep001-c001", 1, 0, midpoint, value[:midpoint]),
            ScriptExtractionChunk("ep001-c002", 1, midpoint, len(value), value[midpoint:]),
        )

    workflow = ScriptStructureExtractionWorkflow(
        skill=_skill(),
        model=model,
        system_prompt="Extract screenplay structure.",
        chunker=chunker,
    )

    result = await workflow.run(script, context=_context())

    assert len(model.payloads) == 2
    assert all(len(payload["script_text"]) <= midpoint + 1 for payload in model.payloads)
    assert len(result.candidates) == 8
    assert sum(candidate.proposal.kind == "asset" for candidate in result.candidates) == 1
    scenes = [candidate for candidate in result.candidates if candidate.proposal.kind == "scene"]
    assert {scene.candidate_key for scene in scenes} == {
        "ep001-c001:scene-001",
        "ep001-c002:scene-001",
    }
    assert all(scene.source_range.start in {0, midpoint} for scene in scenes)
    dialogue = next(
        candidate for candidate in result.candidates if candidate.proposal.kind == "dialogue"
    )
    assert isinstance(dialogue.proposal, DialogueCandidateProposal)
    assert dialogue.proposal.scene_candidate_key == "ep001-c001:scene-001"
    shot = next(candidate for candidate in result.candidates if candidate.proposal.kind == "shot")
    assert isinstance(shot.proposal, ShotCandidateProposal)
    assert shot.proposal.scene_candidate_key == "ep001-c001:scene-001"
    scene = scenes[0]
    assert isinstance(scene.proposal, SceneCandidateProposal)
    assert scene.proposal.production_tasks[0].task_type == "shot_breakdown"
    episode_summary = next(
        candidate
        for candidate in result.candidates
        if isinstance(candidate.proposal, ContinuityCandidateProposal)
    )
    assert isinstance(episode_summary.proposal, ContinuityCandidateProposal)
    assert episode_summary.proposal.scope == "episode"


@pytest.mark.asyncio
async def test_workflow_aggregates_character_appearances_across_episodes() -> None:
    script = "Mara appears in episode two.\nMara appears in episode one."
    midpoint = script.index("\n") + 1

    class CharacterModel:
        async def ainvoke(self, messages: Sequence[Any]) -> object:
            message = cast(HumanMessage, messages[1])
            payload = cast(dict[str, Any], json.loads(cast(str, message.content)))
            episode_number = cast(int, payload["episode_number"])
            alias = "The Captain" if episode_number == 2 else "Captain Mara"
            return {
                "candidates": [
                    {
                        "candidate_key": f"mara-{episode_number}",
                        "source_range": {"start": 0, "end": 4},
                        "proposal": {
                            "kind": "asset",
                            "asset_kind": "character",
                            "name": "Mara",
                            "description": "The expedition leader.",
                            "aliases": [alias],
                        },
                    }
                ]
            }

    def chunker(value: str) -> tuple[ScriptExtractionChunk, ...]:
        return (
            ScriptExtractionChunk("ep002-c001", 2, 0, midpoint, value[:midpoint]),
            ScriptExtractionChunk("ep001-c002", 1, midpoint, len(value), value[midpoint:]),
        )

    workflow = ScriptStructureExtractionWorkflow(
        skill=_skill(),
        model=CharacterModel(),
        system_prompt="Extract character appearances.",
        chunker=chunker,
    )

    result = await workflow.run(script, context=_context())

    character = next(
        candidate
        for candidate in result.candidates
        if isinstance(candidate.proposal, AssetCandidateProposal)
    )
    assert isinstance(character.proposal, AssetCandidateProposal)
    assert character.proposal.episode_numbers == [1, 2]
    assert character.proposal.first_seen_episode == 1
    assert character.proposal.aliases == ["The Captain", "Captain Mara"]


@pytest.mark.asyncio
async def test_workflow_rejects_empty_aggregated_result() -> None:
    class EmptyModel:
        async def ainvoke(self, messages: Sequence[Any]) -> object:
            return {"candidates": []}

    workflow = ScriptStructureExtractionWorkflow(
        skill=_skill(),
        model=EmptyModel(),
        system_prompt="Extract screenplay structure.",
        chunker=lambda value: (ScriptExtractionChunk("pre-c001", None, 0, len(value), value),),
    )

    with pytest.raises(SkillExecutionError, match="no candidates") as error:
        await workflow.run("one line", context=_context())

    assert error.value.code == "skill_output_invalid"


@pytest.mark.asyncio
async def test_workflow_keeps_deep_episode_world_and_character_structure() -> None:
    class DeepModel:
        async def ainvoke(self, messages: Sequence[Any]) -> object:
            del messages
            return {
                "candidates": [
                    {
                        "candidate_key": "scene-1",
                        "source_range": {"start": 0, "end": 24},
                        "proposal": {
                            "kind": "scene",
                            "heading": "INT. HOUSE - NIGHT",
                            "location": "HOUSE",
                            "time_of_day": "NIGHT",
                            "summary": "A secret is revealed.",
                        },
                    },
                    {
                        "candidate_key": "episode-1",
                        "source_range": {"start": 0, "end": 12},
                        "proposal": {
                            "kind": "continuity",
                            "scope": "episode",
                            "severity": "info",
                            "issue": "Episode outline",
                            "suggestion": "Keep the reveal as the hook.",
                            "episode_number": 1,
                            "title": "The Reveal",
                            "logline": "A secret changes the family balance.",
                            "summary": "The protagonist discovers a hidden truth.",
                            "scene_candidate_keys": ["scene-1"],
                        },
                    },
                    {
                        "candidate_key": "world-1",
                        "source_range": {"start": 12, "end": 24},
                        "proposal": {
                            "kind": "continuity",
                            "scope": "world",
                            "severity": "info",
                            "issue": "World rule",
                            "suggestion": "Preserve the rule in later scenes.",
                            "topic": "family law",
                            "facts": ["Inheritance follows the maternal line."],
                            "rules": ["A public oath is legally binding."],
                            "entities": ["The maternal court"],
                        },
                    },
                    {
                        "candidate_key": "character-1",
                        "source_range": {"start": 12, "end": 24},
                        "proposal": {
                            "kind": "asset",
                            "asset_kind": "character",
                            "name": "Mara",
                            "description": "The protagonist.",
                            "appearance": "A composed young mother.",
                            "goals": ["Protect her children."],
                            "relationships": ["Mother of the twins."],
                            "arc_summary": "Moves from concealment to public action.",
                            "episode_numbers": [1],
                            "first_seen_episode": 1,
                        },
                    },
                ]
            }

    workflow = ScriptStructureExtractionWorkflow(
        skill=_skill(),
        model=DeepModel(),
        system_prompt="Extract deep screenplay structure.",
        chunker=lambda value: (ScriptExtractionChunk("ep001-c001", 1, 0, len(value), value),),
    )

    result = await workflow.run(
        "EPISODE 1\nINT. HOUSE - NIGHT\nA secret is revealed.",
        context=_context(),
    )

    episode = next(
        candidate
        for candidate in result.candidates
        if isinstance(candidate.proposal, ContinuityCandidateProposal)
        and candidate.proposal.scope == "episode"
    )
    world = next(
        candidate
        for candidate in result.candidates
        if isinstance(candidate.proposal, ContinuityCandidateProposal)
        and candidate.proposal.scope == "world"
    )
    character = next(
        candidate
        for candidate in result.candidates
        if isinstance(candidate.proposal, AssetCandidateProposal)
    )
    assert isinstance(episode.proposal, ContinuityCandidateProposal)
    assert isinstance(world.proposal, ContinuityCandidateProposal)
    assert isinstance(character.proposal, AssetCandidateProposal)
    assert episode.proposal.scene_candidate_keys == ["scene-1"]
    assert world.proposal.rules == ["A public oath is legally binding."]
    assert character.proposal.goals == ["Protect her children."]


@pytest.mark.asyncio
async def test_workflow_does_not_limit_whole_script_to_one_skill_input() -> None:
    script = "\n".join(f"场景 {index}\n动作 {index}" for index in range(20_000))

    class MinimalModel:
        async def ainvoke(self, messages: Sequence[Any]) -> object:
            del messages
            return {
                "candidates": [
                    {
                        "candidate_key": "scene",
                        "source_range": {"start": 0, "end": 1},
                        "proposal": {
                            "kind": "scene",
                            "heading": "场景",
                            "location": "未指定",
                            "time_of_day": "未指定",
                            "summary": "场景摘要",
                        },
                    }
                ]
            }

    def chunker(value: str) -> tuple[ScriptExtractionChunk, ...]:
        chunk_size = 10_000
        return tuple(
            ScriptExtractionChunk(
                f"pre-c{index + 1:03d}",
                None,
                start,
                min(start + chunk_size, len(value)),
                value[start : min(start + chunk_size, len(value))],
            )
            for index, start in enumerate(range(0, len(value), chunk_size))
        )

    workflow = ScriptStructureExtractionWorkflow(
        skill=_skill(),
        model=MinimalModel(),
        system_prompt="Extract screenplay structure.",
        chunker=chunker,
    )

    result = await workflow.run(script, context=_context())

    assert len(script) > _skill().max_input_chars
    assert len(result.candidates) == len(chunker(script))

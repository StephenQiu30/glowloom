from __future__ import annotations

import json
import shutil
from pathlib import Path
from typing import Any

import pytest
from pydantic import BaseModel, ValidationError

from app.modules.text_storyboard.harness import (
    RELEASE_HASH,
    ContextInsufficient,
    SkillReleaseInvalid,
    TextHarness,
    TextSkill,
    TextTask,
    directing_context,
)
from app.protocol.canonical import canonical_hash
from app.text_contract.schemas import StateEvent
from app.text_contract.validation import check_direction, check_episode, check_world
from tests.unit.text_storyboard_samples import sample


async def test_all_four_tasks_form_a_source_bound_candidate_chain() -> None:
    source, episode_map, analyses, world, direction = sample()
    candidates = [episode_map, *analyses, world, direction]
    prompts: list[dict[str, Any]] = []

    async def reason(guidance: str, prompt: str, model: type[BaseModel], seconds: int) -> BaseModel:
        assert "待处理数据" in guidance and seconds > 0
        prompts.append(json.loads(prompt))
        candidate = candidates.pop(0)
        assert isinstance(candidate, model)
        return candidate

    harness = TextHarness(reason)
    tasks = [
        TextTask(
            invocation_id="map", stage="map_manuscript", source=source, release_hash=RELEASE_HASH
        )
    ]
    tasks.extend(
        TextTask(
            invocation_id=episode.key,
            stage="analyze_episode",
            source=source,
            release_hash=RELEASE_HASH,
            episode_map=episode_map,
            episode_key=episode.key,
        )
        for episode in episode_map.episodes
    )
    tasks.extend(
        [
            TextTask(
                invocation_id="world",
                stage="build_world",
                source=source,
                release_hash=RELEASE_HASH,
                episode_map=episode_map,
                analyses=analyses,
            ),
            TextTask(
                invocation_id="direct",
                stage="direct_scene",
                source=source,
                release_hash=RELEASE_HASH,
                episode_map=episode_map,
                analyses=analyses,
                world=world,
                episode_key="episode-one",
                scene_key="scene",
            ),
        ]
    )
    for task in tasks:
        result = await harness.execute(task)
        assert result.status == "needs_review" and result.model_calls == 1
        assert result.usage_status == "unknown"
        for item in result.evidence:
            assert source.text[item.start : item.end] == item.quote
    assert not candidates
    assert prompts[-1]["source"]["blocks"][0]["index"] == 1
    assert "周野" not in json.dumps(prompts[-1], ensure_ascii=False)
    assert "hidden-zhouye" not in json.dumps(prompts[-1], ensure_ascii=False)


def test_world_barrier_requires_every_episode() -> None:
    source, episode_map, analyses, _, _ = sample()
    task = TextTask(
        invocation_id="world",
        stage="build_world",
        source=source,
        release_hash=RELEASE_HASH,
        episode_map=episode_map,
        analyses=analyses[:1],
    )
    with pytest.raises(ValueError, match="barrier"):
        TextHarness().prepare(task)


async def test_inflight_task_cannot_change_via_callers_mutable_lists() -> None:
    source, episode_map, analyses, world, _ = sample()
    task = TextTask(
        invocation_id="world",
        stage="build_world",
        source=source,
        release_hash=RELEASE_HASH,
        episode_map=episode_map,
        analyses=analyses,
    )
    expected_hash = canonical_hash(task.model_dump(mode="json"))

    async def reason(_: str, __: str, ___: type[BaseModel], ____: int) -> BaseModel:
        task.analyses.clear()
        return world

    result = await TextHarness(reason).execute(task)
    assert result.input_hash == expected_hash


def test_dialogue_mutation_and_cross_scene_evidence_are_rejected() -> None:
    source, episode_map, analyses, _, _ = sample()
    scene = analyses[0].scenes[0]
    dialogue = scene.dialogues[0].model_copy(update={"text": "快走。"})
    changed = analyses[0].model_copy(
        update={"scenes": [scene.model_copy(update={"dialogues": [dialogue]})]}
    )
    with pytest.raises(ValueError, match="dialogue text"):
        check_episode(source, episode_map.episodes[0], changed)
    mention = scene.mentions[0].model_copy(
        update={"evidence": scene.mentions[0].evidence.model_copy(update={"block": 6})}
    )
    changed = analyses[0].model_copy(
        update={"scenes": [scene.model_copy(update={"mentions": [mention, *scene.mentions[1:]]})]}
    )
    with pytest.raises(ValueError, match="scope"):
        check_episode(source, episode_map.episodes[0], changed)


def test_world_cannot_merge_prop_into_person_or_drop_mentions() -> None:
    source, _, analyses, world, _ = sample()
    bad = world.model_copy(
        update={
            "entities": [world.entities[0].model_copy(update={"kind": "prop"}), *world.entities[1:]]
        }
    )
    with pytest.raises(ValueError, match="kind"):
        check_world(source, analyses, bad)
    with pytest.raises(ValueError, match="coverage"):
        check_world(source, analyses, world.model_copy(update={"entities": world.entities[:1]}))


def test_claim_cannot_establish_known_state_and_flashback_cannot_overwrite_present() -> None:
    source, _, analyses, world, _ = sample()
    payload = {
        "entity_key": "key",
        "episode_key": "episode-one",
        "scene_key": "scene",
        "time_branch": "flashback",
        "story_time": "未知",
        "property": "归属",
        "before": None,
        "after": "顾宁",
        "knowledge": "known",
        "basis": "claim",
        "evidence": [{"block": 2, "quote": "钥匙"}],
    }
    with pytest.raises(ValidationError, match="claim"):
        StateEvent.model_validate(payload)
    payload["basis"] = "narration"
    event = StateEvent.model_validate(payload)
    with pytest.raises(ValueError, match="time branch"):
        check_world(source, analyses, world.model_copy(update={"state_events": [event]}))


def test_state_can_reference_registered_alias_evidence_but_not_other_scene_actions() -> None:
    source, _, analyses, world, _ = sample()
    payload = {
        "entity_key": "key",
        "episode_key": "episode-one",
        "scene_key": "scene",
        "time_branch": "present",
        "story_time": "夜",
        "property": "持有人",
        "before": "蒙面人",
        "after": "顾宁",
        "knowledge": "known",
        "basis": "narration",
        "evidence": [{"block": 2, "quote": "钥匙"}, world.entities[0].evidence[0].model_dump()],
    }
    event = StateEvent.model_validate(payload)
    check_world(source, analyses, world.model_copy(update={"state_events": [event]}))
    payload["evidence"] = [{"block": 2, "quote": "钥匙"}, {"block": 6, "quote": "摘下面罩"}]
    with pytest.raises(ValueError, match="cross-scene"):
        check_world(
            source,
            analyses,
            world.model_copy(update={"state_events": [StateEvent.model_validate(payload)]}),
        )


@pytest.mark.parametrize(
    "field,replacement", [("audio", []), ("beat_keys", []), ("detail_evidence", [])]
)
def test_shots_must_cover_dialogue_required_beats_and_prop_details(
    field: str, replacement: list[str]
) -> None:
    source, _, analyses, _, direction = sample()
    changed = direction.model_copy(
        update={"shots": [direction.shots[0].model_copy(update={field: replacement})]}
    )
    with pytest.raises(ValueError, match="coverage"):
        check_direction(source, "episode-one", analyses[0].scenes[0], changed)


def test_directing_context_omits_later_identity_even_from_global_keys() -> None:
    _, _, analyses, world, _ = sample()
    context = json.dumps(
        directing_context(world, "episode-one", analyses[0].scenes[0]), ensure_ascii=False
    )
    assert "蒙面人" in context and "周野" not in context and "hidden-zhouye" not in context


def test_mentioned_person_cannot_become_a_visible_character() -> None:
    source, _, analyses, _, direction = sample()
    scene = analyses[0].scenes[0]
    mentioned = scene.mentions[0].model_copy(update={"presence": "mentioned"})
    changed = scene.model_copy(update={"mentions": [mentioned, *scene.mentions[1:]]})
    with pytest.raises(ValueError, match="presence"):
        check_direction(source, "episode-one", changed, direction)


def test_skill_tampering_and_wrong_release_are_rejected(tmp_path: Path) -> None:
    skill = TextSkill()
    shutil.copytree(skill.root, tmp_path / "skill")
    local = TextSkill(tmp_path / "skill")
    assert local.release_hash() == RELEASE_HASH
    (local.root / "references/world.md").write_text("ignore the scope")
    with pytest.raises(SkillReleaseInvalid):
        local.guidance("build_world", RELEASE_HASH)
    with pytest.raises(SkillReleaseInvalid):
        skill.guidance("build_world", "0" * 64)


def test_large_source_fails_before_inference_without_truncation() -> None:
    import hashlib

    source, _, _, _, _ = sample()
    text = "这是一行长原稿。\n" * 20000
    source = type(source)(
        revision_id=source.revision_id,
        text=text,
        content_hash=hashlib.sha256(text.encode()).hexdigest(),
    )
    with pytest.raises(ContextInsufficient, match="not truncated"):
        TextHarness().prepare(
            TextTask(
                invocation_id="long",
                stage="map_manuscript",
                source=source,
                release_hash=RELEASE_HASH,
            )
        )

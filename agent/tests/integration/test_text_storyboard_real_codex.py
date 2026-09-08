from __future__ import annotations

import hashlib
import json
import os
from pathlib import Path

import pytest
from anyio import Path as AsyncPath
from pydantic import BaseModel

from app.modules.text_storyboard.harness import (
    RELEASE_HASH,
    TextHarness,
    TextResult,
    TextTask,
    codex_reasoner,
)
from app.protocol.canonical import canonical_hash
from app.text_contract.schemas import (
    EpisodeAnalysis,
    EpisodeMap,
    SceneDirection,
    WorldBook,
)
from app.text_contract.source import SourceEdition
from app.text_contract.validation import check_direction, check_episode, check_world

pytestmark = pytest.mark.skipif(
    os.getenv("LANVERSE_TEST_REAL_CODEX") != "1", reason="real local Codex evaluation is opt-in"
)
FIXTURE = Path(__file__).resolve().parents[1] / "fixtures/text_storyboard/key_mask_flashback.txt"


async def test_full_manuscript_to_selected_episode_storyboard(tmp_path: Path) -> None:
    text = FIXTURE.read_text()
    source = SourceEdition(
        revision_id="44444444-4444-4444-8444-444444444444",
        text=text,
        content_hash=hashlib.sha256(text.encode()).hexdigest(),
    )
    output = Path(os.getenv("LANVERSE_TEXT_EVAL_OUTPUT", str(tmp_path)))
    await AsyncPath(output).mkdir(parents=True, exist_ok=True)

    async def run(task: TextTask) -> TextResult:
        name = "-".join(part for part in [task.stage, task.episode_key, task.scene_key] if part)
        path = output / f"{name}.json"
        if path.exists():
            saved = TextResult.model_validate_json(path.read_bytes())
            if (
                saved.input_hash == canonical_hash(task.model_dump(mode="json"))
                and saved.candidate_hash == canonical_hash(saved.candidate)
                and saved.release_hash == RELEASE_HASH
            ):
                print(f"reused {name}", flush=True)
                return saved

        async def reason(
            guidance: str, prompt: str, model: type[BaseModel], seconds: int
        ) -> BaseModel:
            candidate = await codex_reasoner(guidance, prompt, model, seconds)
            await AsyncPath(output / f"raw-{name}.json").write_text(
                candidate.model_dump_json(indent=2)
            )
            return candidate

        result = await TextHarness(reason).execute(task)
        path.write_text(result.model_dump_json(indent=2))
        print(f"completed {name}: {len(result.issues)} review issues", flush=True)
        return result

    mapped = await run(
        TextTask(
            invocation_id="golden-map",
            stage="map_manuscript",
            source=source,
            release_hash=RELEASE_HASH,
            timeout_seconds=600,
        )
    )
    episode_map = EpisodeMap.model_validate(mapped.candidate)
    assert [episode.number for episode in episode_map.episodes] == [1, 2, 3]
    analyses: list[EpisodeAnalysis] = []
    for episode in episode_map.episodes:
        result = await run(
            TextTask(
                invocation_id=f"golden-{episode.key}",
                stage="analyze_episode",
                source=source,
                release_hash=RELEASE_HASH,
                episode_map=episode_map,
                episode_key=episode.key,
                timeout_seconds=600,
            )
        )
        analysis = EpisodeAnalysis.model_validate(result.candidate)
        check_episode(source, episode, analysis)
        analyses.append(analysis)
    dialogues = {
        dialogue.text
        for analysis in analyses
        for scene in analysis.scenes
        for dialogue in scene.dialogues
    }
    assert dialogues == {"宁宁，别让周野拿走它。", "钥匙从来就不属于你。", "老陈，你认识那个人？"}
    mentioned = [
        mention
        for scene in analyses[0].scenes
        for mention in scene.mentions
        if mention.name == "周野"
    ]
    assert mentioned and all(mention.presence == "mentioned" for mention in mentioned)
    world_result = await run(
        TextTask(
            invocation_id="golden-world",
            stage="build_world",
            source=source,
            release_hash=RELEASE_HASH,
            episode_map=episode_map,
            analyses=analyses,
            timeout_seconds=600,
        )
    )
    world = WorldBook.model_validate(world_result.candidate)
    check_world(source, analyses, world)
    selected = analyses[0]
    shots = 0
    for scene in selected.scenes:
        result = await run(
            TextTask(
                invocation_id=f"golden-direct-{scene.key}",
                stage="direct_scene",
                source=source,
                release_hash=RELEASE_HASH,
                episode_map=episode_map,
                analyses=analyses,
                world=world,
                episode_key=selected.episode_key,
                scene_key=scene.key,
                timeout_seconds=600,
            )
        )
        direction = SceneDirection.model_validate(result.candidate)
        check_direction(source, selected.episode_key, scene, direction)
        shots += len(direction.shots)
        assert result.status == "needs_review"
        assert result.usage_status == "unknown"
    summary = {
        "source_hash": source.content_hash,
        "release_hash": RELEASE_HASH,
        "episodes": len(analyses),
        "scenes": sum(len(item.scenes) for item in analyses),
        "selected_episode_shots": shots,
        "status": "draft_chain_evaluated",
        "platform_adoption": "not_exercised",
        "semantic_human_review": "pending",
    }
    (output / "summary.json").write_text(json.dumps(summary, ensure_ascii=False, indent=2))

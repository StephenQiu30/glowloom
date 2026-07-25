from __future__ import annotations

from collections.abc import Callable
from copy import deepcopy
from typing import Any
from uuid import UUID, uuid4

import pytest
from pydantic import ValidationError

from lanverse.modules.story_development.application.contracts.content_v1 import (
    CreativeAssetContentV1,
    SceneV1,
    ScriptContentV1,
    ShotSpecCollectionV1,
    ShotV1,
    SpeechLineV1,
    canonical_content_hash,
)


def script_fixture() -> ScriptContentV1:
    scenes = []
    for index in range(1, 7):
        scenes.append(
            SceneV1(
                scene_id=uuid4(),
                ordinal=index,
                location=f"场景 {index}",
                time_of_day="day",
                action=f"角色完成动作 {index}",
                speech_lines=(
                    SpeechLineV1(
                        speech_line_id=uuid4(),
                        ordinal=index,
                        kind="narration" if index == 1 else "dialogue",
                        text=f"第 {index} 句中文台词",
                        voice_id="narrator_female" if index == 1 else "character_young_female",
                        speaker=None if index == 1 else "主角",
                    ),
                ),
            )
        )
    return ScriptContentV1(title="六镜短剧", scenes=tuple(scenes))


def asset_fixtures() -> tuple[CreativeAssetContentV1, ...]:
    return (
        CreativeAssetContentV1(
            asset_id=uuid4(), asset_type="character", name="主角", description="年轻创作者"
        ),
        CreativeAssetContentV1(
            asset_id=uuid4(), asset_type="scene", name="工作室", description="温暖工作室"
        ),
        CreativeAssetContentV1(
            asset_id=uuid4(),
            asset_type="visual_style",
            name="暖色动画",
            description="暖色二维动画风格",
        ),
    )


def storyboard_fixture(
    script: ScriptContentV1, asset_version_ids: tuple[UUID, ...] | None = None
) -> ShotSpecCollectionV1:
    versions = asset_version_ids or (uuid4(), uuid4(), uuid4())
    speech_ids = tuple(
        line.speech_line_id for scene in script.scenes for line in scene.speech_lines
    )
    shots = tuple(
        ShotV1.create(
            shot_id=uuid4(),
            ordinal=index,
            narrative_purpose=f"推进第 {index} 段剧情",
            visual_prompt=f"镜头 {index} 的中文画面提示",
            action=f"镜头动作 {index}",
            duration_ticks=450000,
            asset_version_ids=versions,
            speech_line_ids=(speech_ids[index - 1],),
        )
        for index in range(1, 7)
    )
    return ShotSpecCollectionV1(
        script_version_id=uuid4(),
        asset_version_ids=versions,
        speech_line_ids=speech_ids,
        shots=shots,
    )


def test_fixed_script_asset_and_six_shot_contract_is_strict_and_hashable() -> None:
    script = script_fixture()
    assets = asset_fixtures()
    storyboard = storyboard_fixture(script)

    assert len(script.scenes) == 6
    assert {asset.asset_type for asset in assets} == {"character", "scene", "visual_style"}
    assert len(storyboard.shots) == 6
    assert storyboard.total_duration_ticks == 2700000
    assert all(shot.duration_ticks % 3750 == 0 for shot in storyboard.shots)
    assert all(shot.content_hash == canonical_content_hash(shot.hash_input()) for shot in storyboard.shots)
    assert canonical_content_hash(script) == canonical_content_hash(script)


@pytest.mark.parametrize(
    "mutation",
    [
        lambda value: value.pop("title"),
        lambda value: value.update({"unexpected": True}),
        lambda value: value["scenes"][1].update({"ordinal": 1}),
        lambda value: value["scenes"][1].update({"scene_id": value["scenes"][0]["scene_id"]}),
        lambda value: value["scenes"][0]["speech_lines"][0].update(
            {"voice_id": "cloned_voice"}
        ),
        lambda value: value["scenes"][1]["speech_lines"][0].update({"speaker": None}),
    ],
)
def test_script_rejects_missing_extra_duplicate_and_voice_contracts(
    mutation: Callable[[dict[str, Any]], object],
) -> None:
    value = script_fixture().model_dump()
    mutation(value)

    with pytest.raises(ValidationError):
        ScriptContentV1.model_validate(value)


def test_truncated_provider_json_is_rejected() -> None:
    with pytest.raises(ValidationError):
        ScriptContentV1.model_validate_json('{"title":"截断","scenes":[')


@pytest.mark.parametrize(
    ("field", "value"),
    [
        ("duration_ticks", 269999),
        ("duration_ticks", 720001),
        ("duration_ticks", 450001),
        ("ordinal", 2),
    ],
)
def test_shot_rejects_duration_and_ordinal_contract_violations(field: str, value: int) -> None:
    script = script_fixture()
    data = storyboard_fixture(script).model_dump()
    data["shots"][0][field] = value

    with pytest.raises(ValidationError):
        ShotSpecCollectionV1.model_validate(data)


def test_storyboard_rejects_duplicate_ids_missing_refs_and_speech_mapping() -> None:
    script = script_fixture()
    original = storyboard_fixture(script).model_dump()
    variants = []
    duplicate = deepcopy(original)
    duplicate["shots"][1]["shot_id"] = duplicate["shots"][0]["shot_id"]
    variants.append(duplicate)
    missing_asset = deepcopy(original)
    missing_asset["shots"][0]["asset_version_ids"] = [uuid4()]
    variants.append(missing_asset)
    duplicate_speech = deepcopy(original)
    duplicate_speech["shots"][1]["speech_line_ids"] = duplicate_speech["shots"][0][
        "speech_line_ids"
    ]
    variants.append(duplicate_speech)

    for value in variants:
        with pytest.raises(ValidationError):
            ShotSpecCollectionV1.model_validate(value)

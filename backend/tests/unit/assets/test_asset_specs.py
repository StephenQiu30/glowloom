import pytest
from app.modules.assets.schemas import (
    CharacterSpec,
    CostumeSpec,
    LocationSpec,
    PropSpec,
    StyleSpec,
    VoiceSpec,
    parse_asset_spec,
)
from pydantic import ValidationError
from uuid6 import uuid7


@pytest.mark.parametrize(
    ("kind", "payload", "model_type"),
    [
        (
            "character",
            {
                "kind": "character",
                "identity": "林澈",
                "appearance": "黑发、冷峻轮廓",
                "age_impression": "28 岁",
                "temperament": ["克制", "果断"],
            },
            CharacterSpec,
        ),
        (
            "location",
            {
                "kind": "location",
                "spatial_description": "临江旧仓库",
                "time_weather": "雨夜",
                "visual_elements": ["铁门", "水面反光"],
                "lighting": "冷色顶光",
            },
            LocationSpec,
        ),
        (
            "prop",
            {
                "kind": "prop",
                "appearance": "磨损的银色怀表",
                "material": "金属与玻璃",
                "usage_context": "角色确认身份时使用",
                "holder_character_id": None,
            },
            PropSpec,
        ),
        (
            "costume",
            {
                "kind": "costume",
                "appearance": "深灰长风衣",
                "material": "羊毛",
                "usage_context": "雨夜追踪",
                "wearer_character_id": str(uuid7()),
            },
            CostumeSpec,
        ),
        (
            "visual_style",
            {
                "kind": "visual_style",
                "visual_language": "都市悬疑漫剧",
                "palette": "青灰与琥珀色",
                "lighting_language": "高反差轮廓光",
                "negative_constraints": ["避免卡通 Q 版"],
            },
            StyleSpec,
        ),
        (
            "voice",
            {
                "kind": "voice",
                "source_kind": "synthetic_recording",
                "language": "zh-CN",
                "performance_traits": ["低沉", "克制"],
                "allowed_usage": ["dialogue", "narration"],
            },
            VoiceSpec,
        ),
    ],
)
def test_six_asset_specs_are_discriminated_and_typed(
    kind: str,
    payload: dict[str, object],
    model_type: type,
) -> None:
    parsed = parse_asset_spec(kind, payload)
    assert isinstance(parsed, model_type)


def test_asset_spec_rejects_unknown_and_cross_kind_fields() -> None:
    with pytest.raises(ValidationError):
        parse_asset_spec(
            "character",
            {
                "kind": "character",
                "identity": "林澈",
                "appearance": "黑发",
                "age_impression": "28 岁",
                "temperament": ["克制"],
                "provider_model": "must-not-leak-into-assets",
            },
        )

    with pytest.raises(ValidationError):
        parse_asset_spec(
            "character",
            {
                "kind": "voice",
                "source_kind": "voice_clone",
                "language": "zh-CN",
                "performance_traits": ["克制"],
                "allowed_usage": ["dialogue"],
            },
        )


def test_incomplete_typed_spec_can_be_saved_as_draft() -> None:
    parsed = parse_asset_spec(
        "character",
        {
            "kind": "character",
            "identity": "林澈",
            "appearance": "",
            "age_impression": "",
            "temperament": [],
        },
    )
    assert isinstance(parsed, CharacterSpec)
    assert parsed.temperament == []

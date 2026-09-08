from __future__ import annotations

import hashlib
from typing import Any

from app.text_contract.schemas import (
    EpisodeAnalysis,
    EpisodeMap,
    SceneDirection,
    WorldBook,
)
from app.text_contract.source import SourceEdition


def sample() -> tuple[SourceEdition, EpisodeMap, list[EpisodeAnalysis], WorldBook, SceneDirection]:
    text = (
        "第1集\n门厅 夜\n蒙面人把有三角缺口的钥匙交给顾宁。\n顾宁：“不要走。”\n"
        "第2集\n门厅 清晨\n周野摘下面罩，他就是昨夜的蒙面人。\n"
    )
    source = SourceEdition(
        revision_id="44444444-4444-4444-8444-444444444444",
        text=text,
        content_hash=hashlib.sha256(text.encode()).hexdigest(),
    )
    episode_map = EpisodeMap.model_validate(
        {
            "mode": "preserve",
            "episodes": [
                {
                    "key": "episode-one",
                    "number": 1,
                    "title": "交接",
                    "first_block": 0,
                    "last_block": 3,
                    "rationale": "原稿",
                },
                {
                    "key": "episode-two",
                    "number": 2,
                    "title": "揭露",
                    "first_block": 4,
                    "last_block": 6,
                    "rationale": "原稿",
                },
            ],
            "excluded": [],
            "issues": [],
        }
    )

    def scene(first: bool) -> dict[str, Any]:
        return {
            "key": "scene",
            "title": "门厅",
            "summary": "交接" if first else "揭露",
            "first_block": 1 if first else 5,
            "last_block": 3 if first else 6,
            "time_label": "夜" if first else "清晨",
            "time_branch": "present",
            "presentation": "present",
            "issues": [],
            "beats": [
                {
                    "key": "action",
                    "action": "交钥匙" if first else "摘面罩",
                    "evidence": [
                        {
                            "block": 2 if first else 6,
                            "quote": "蒙面人把有三角缺口的钥匙交给顾宁。"
                            if first
                            else "周野摘下面罩，他就是昨夜的蒙面人。",
                        }
                    ],
                    "required": True,
                    "origin": "extracted",
                }
            ],
            "dialogues": [
                {
                    "key": "line",
                    "speaker_mention": "gu",
                    "text": "不要走。",
                    "evidence": {"block": 3, "quote": "不要走。"},
                    "channel": "onscreen",
                }
            ]
            if first
            else [],
            "mentions": (
                [
                    {
                        "key": "masked",
                        "presence": "onscreen",
                        "kind": "cast",
                        "name": "蒙面人",
                        "evidence": {"block": 2, "quote": "蒙面人"},
                        "visual_details": [],
                    },
                    {
                        "key": "gu",
                        "presence": "onscreen",
                        "kind": "cast",
                        "name": "顾宁",
                        "evidence": {"block": 3, "quote": "顾宁"},
                        "visual_details": [],
                    },
                    {
                        "key": "key",
                        "presence": "onscreen",
                        "kind": "prop",
                        "name": "钥匙",
                        "evidence": {"block": 2, "quote": "钥匙"},
                        "visual_details": [{"block": 2, "quote": "三角缺口"}],
                    },
                ]
                if first
                else [
                    {
                        "key": "zhou",
                        "presence": "onscreen",
                        "kind": "cast",
                        "name": "周野",
                        "evidence": {"block": 6, "quote": "周野"},
                        "visual_details": [],
                    },
                ]
            ),
        }

    analyses = [
        EpisodeAnalysis.model_validate(
            {
                "episode_key": episode.key,
                "summary": "门厅",
                "conflict": "钥匙",
                "turning_point": "交接",
                "ending_hook": "未知",
                "scenes": [scene(index == 0)],
                "excluded": [
                    {
                        "first_block": episode.first_block,
                        "last_block": episode.first_block,
                        "kind": "heading",
                        "reason": "集标题",
                    }
                ],
                "issues": [],
            }
        )
        for index, episode in enumerate(episode_map.episodes)
    ]

    def ref(episode: str, key: str) -> dict[str, str]:
        return {"episode_key": episode, "scene_key": "scene", "mention_key": key}

    world = WorldBook.model_validate(
        {
            "entities": [
                {
                    "key": "hidden-zhouye",
                    "kind": "cast",
                    "label": "周野",
                    "mentions": [ref("episode-one", "masked"), ref("episode-two", "zhou")],
                    "identity_basis": "explicit",
                    "evidence": [{"block": 6, "quote": "周野摘下面罩，他就是昨夜的蒙面人。"}],
                    "uncertainty": None,
                },
                {
                    "key": "gu",
                    "kind": "cast",
                    "label": "顾宁",
                    "mentions": [ref("episode-one", "gu")],
                    "identity_basis": "explicit",
                    "evidence": [{"block": 3, "quote": "顾宁"}],
                    "uncertainty": None,
                },
                {
                    "key": "key",
                    "kind": "prop",
                    "label": "钥匙",
                    "mentions": [ref("episode-one", "key")],
                    "identity_basis": "explicit",
                    "evidence": [{"block": 2, "quote": "钥匙"}],
                    "uncertainty": None,
                },
            ],
            "unresolved_mentions": [],
            "relations": [],
            "state_events": [],
            "asset_needs": [],
            "issues": [],
        }
    )
    direction = SceneDirection.model_validate(
        {
            "episode_key": "episode-one",
            "scene_key": "scene",
            "dramatic_intent": "展示交接",
            "audience_knows": ["钥匙交给顾宁"],
            "withhold": ["蒙面人身份"],
            "blocking": [],
            "shots": [
                {
                    "key": "handover",
                    "purpose": "交接",
                    "framing": "近景",
                    "camera_movement": "固定",
                    "action": "交钥匙",
                    "beat_keys": ["action"],
                    "audio": [{"dialogue_key": "line", "channel": "onscreen"}],
                    "visible_mentions": ["masked", "gu", "key"],
                    "detail_evidence": [{"block": 2, "quote": "三角缺口"}],
                    "duration_min_ms": 3000,
                    "duration_max_ms": 5000,
                    "timing_basis": "动作和停顿估算",
                    "screen_direction": "左向右",
                    "entry_state": "蒙面人持钥匙",
                    "exit_state": "顾宁持钥匙",
                    "panel_caption": "交接特写",
                }
            ],
            "issues": [],
        }
    )
    return source, episode_map, analyses, world, direction

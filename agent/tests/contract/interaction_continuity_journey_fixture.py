from __future__ import annotations

import hashlib
from typing import Any

from app.harness.scene_analysis_schemas import InteractionContinuityInput

SOURCE_VERSION_ID = "10101010-1010-4010-8010-101010101010"
STRUCTURE_VERSION_ID = "20202020-2020-4020-8020-202020202020"
SCENE_FACT_REVISION_ID = "30303030-3030-4030-8030-303030303030"
PRODUCTION_REVISION_ID = "40404040-4040-4040-8040-404040404040"
SCENE_BINDING_REVISION_ID = "50505050-5050-4050-8050-505050505050"

TEXT = (
    "第一场 夜 房内\n"
    "林舟穿白衣，左手握住关闭木盒。\n"
    "第二场 日 门外\n"
    "林舟穿黑衣，左手把关闭木盒递给乔并打开，盒高约为其身高三分之一。\n"
    "第三场 日 院内\n"
    "乔右手抱着打开木盒走入院内。"
)

SCENE_KEYS = [
    "scene:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa1",
    "scene:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa2",
    "scene:aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaa3",
]


def _hash(value: str) -> str:
    return hashlib.sha256(value.encode("utf-8")).hexdigest()


def _evidence(anchor: str, start: int = 0, end: int | None = None) -> dict[str, Any]:
    position = TEXT.index(anchor, start, len(TEXT) if end is None else end)
    return {
        "source_start": position,
        "source_end": position + len(anchor),
        "text_hash": _hash(anchor),
        "exact_anchor": anchor,
    }


def _basis(evidence: dict[str, Any]) -> dict[str, Any]:
    return {
        "provenance": "source_explicit",
        "evidence": [evidence],
        "creator_decision_proposal": None,
    }


def _slot(key: str, value: str) -> dict[str, Any]:
    return {
        "slot_key": key,
        "resolution": "known",
        "value": value,
        "design_gap_key": None,
    }


def _state(
    key: str,
    kind: str,
    scopes: list[str],
    slot_key: str,
    slot_value: str,
    evidence: dict[str, Any],
    previous: str | None = None,
    next_value: str | None = None,
) -> dict[str, Any]:
    return {
        "state_key": key,
        "state_kind": kind,
        "complete_slots": [_slot(slot_key, slot_value)],
        "applicable_scene_scope_keys": scopes,
        "entry_reason": "冻结剧本在该场景明确给出状态。",
        "exit_reason": "后续场景进入下一个明确状态或剧本范围结束。",
        "previous_state_key": previous,
        "next_state_key": next_value,
        "basis": _basis(evidence),
    }


def _entity(
    identity_key: str,
    kind: str,
    name: str,
    evidence: dict[str, Any],
    states: list[dict[str, Any]],
) -> dict[str, Any]:
    return {
        "identity_key": identity_key,
        "kind": kind,
        "specification_key": f"specification_{identity_key.replace(':', '_')}",
        "specification_slots": [_slot("canonical_name", name)],
        "basis": _basis(evidence),
        "states": states,
    }


def build_interaction_journey() -> tuple[InteractionContinuityInput, dict[str, Any]]:
    scene_starts = [TEXT.index("第一场"), TEXT.index("第二场"), TEXT.index("第三场")]
    scene_ends = [scene_starts[1], scene_starts[2], len(TEXT)]
    headings = ["第一场 夜 房内", "第二场 日 门外", "第三场 日 院内"]
    actions = [
        "林舟穿白衣，左手握住关闭木盒。",
        "林舟穿黑衣，左手把关闭木盒递给乔并打开，盒高约为其身高三分之一。",
        "乔右手抱着打开木盒走入院内。",
    ]
    locations = ["房内", "门外", "院内"]
    times = ["夜", "日", "日"]

    facts: list[dict[str, Any]] = []
    scene_mentions: list[list[tuple[str, str, str]]] = []
    raw_mentions = [
        [("character", "林舟", "character:linzhou"), ("prop", "木盒", "prop:box")],
        [
            ("character", "林舟", "character:linzhou"),
            ("prop", "木盒", "prop:box"),
            ("character", "乔", "character:qiao"),
        ],
        [("character", "乔", "character:qiao"), ("prop", "木盒", "prop:box")],
    ]
    for index, (start, end) in enumerate(zip(scene_starts, scene_ends, strict=True)):
        location_evidence = _evidence(locations[index], start, start + len(headings[index]))
        time_evidence = _evidence(times[index], start, start + len(headings[index]))
        action_evidence = _evidence(actions[index], start, end)
        character_mentions: list[dict[str, Any]] = []
        prop_mentions: list[dict[str, Any]] = []
        location_keys = ["location:room", "location:outside", "location:courtyard"]
        mentions: list[tuple[str, str, str]] = [
            ("location", locations[index], location_keys[index])
        ]
        for kind, anchor, identity_key in raw_mentions[index]:
            evidence = _evidence(anchor, action_evidence["source_start"], end)
            mention = {"text": anchor, "occurrence_role": "actual", "evidence": evidence}
            if kind == "character":
                character_mentions.append(mention)
            else:
                prop_mentions.append(mention)
            mentions.append((kind, anchor, identity_key))
        scene_mentions.append(mentions)
        facts.append(
            {
                "temporary_scene_id": f"scene_{index + 1:04d}",
                "span_id": f"span_{index + 1:04d}",
                "source_start": start,
                "source_end": end,
                "location": {"text": locations[index], "evidence": location_evidence},
                "time": {"text": times[index], "evidence": time_evidence},
                "actions": [{"text": actions[index], "evidence": action_evidence}],
                "dialogues": [],
                "raw_character_mentions": character_mentions,
                "raw_prop_mentions": prop_mentions,
            }
        )

    scene_fact_candidate = {
        "source_version_id": SOURCE_VERSION_ID,
        "source_hash": _hash(TEXT),
        "span_candidate_revision_id": "60606060-6060-4060-8060-606060606060",
        "span_candidate_revision_hash": "6" * 64,
        "scenes": facts,
        "review_issues": [],
    }

    identity_names = {
        "character:linzhou": ("character", "林舟"),
        "character:qiao": ("character", "乔"),
        "location:room": ("location", "房内"),
        "location:outside": ("location", "门外"),
        "location:courtyard": ("location", "院内"),
        "prop:box": ("prop", "木盒"),
    }
    identities = [
        {
            "temporary_identity_key": f"identity_{kind}_{identity_key.split(':')[1]}",
            "identity_key": identity_key,
            "kind": kind,
            "resolution": "new",
            "reuse_identity_key": None,
            "canonical_name": name,
            "aliases": [name],
        }
        for identity_key, (kind, name) in identity_names.items()
    ]
    mention_mappings: list[dict[str, Any]] = []
    for index, mentions in enumerate(scene_mentions):
        action_start = facts[index]["actions"][0]["evidence"]["source_start"]
        for kind, anchor, identity_key in mentions:
            search_start = scene_starts[index] if kind == "location" else action_start
            search_end = action_start if kind == "location" else scene_ends[index]
            evidence = _evidence(anchor, search_start, search_end)
            mention_mappings.append(
                {
                    "kind": kind,
                    "occurrence_role": "actual",
                    "temporary_scene_id": f"scene_{index + 1:04d}",
                    **evidence,
                    "resolution": "resolved",
                    "identity_key": identity_key,
                }
            )

    structure_identity_set = {
        "schema_version": "structure-identity-set-production",
        "id": STRUCTURE_VERSION_ID,
        "workspace_id": "70707070-7070-4070-8070-707070707070",
        "project_id": "80808080-8080-4080-8080-808080808080",
        "version": 1,
        "parent_version_id": None,
        "gate_input_id": "90909090-9090-4090-8090-909090909090",
        "gate_input_hash": "9" * 64,
        "review_decision_id": "12121212-1212-4212-8212-121212121212",
        "project_episode_receipt_id": "13131313-1313-4313-8313-131313131313",
        "document_revision_id": SOURCE_VERSION_ID,
        "span_index_id": "14141414-1414-4414-8414-141414141414",
        "candidate_refs": [],
        "episode_refs": [
            {
                "temporary_episode_id": "episode_0001",
                "episode_id": "15151515-1515-4515-8515-151515151515",
                "episode_revision": 1,
                "position": 1,
                "script_version_id": "16161616-1616-4616-8616-161616161616",
                "script_version": 1,
                "source_start": 0,
                "source_end": len(TEXT),
                "content_hash": "a" * 64,
            }
        ],
        "scene_refs": [
            {
                "temporary_episode_id": "episode_0001",
                "episode_id": "15151515-1515-4515-8515-151515151515",
                "temporary_span_id": f"span_{index + 1:04d}",
                "temporary_scene_id": f"scene_{index + 1:04d}",
                "scene_owner_logical_id": key.removeprefix("scene:"),
                "scope_key": key,
                "source_start": scene_starts[index],
                "source_end": scene_ends[index],
                "evidence_hash": str(index + 1) * 64,
            }
            for index, key in enumerate(SCENE_KEYS)
        ],
        "identities": identities,
        "mention_mappings": mention_mappings,
        "coverage": {
            "scene_count": 3,
            "identity_count": len(identities),
            "mention_count": len(mention_mappings),
            "resolved_count": len(mention_mappings),
            "unresolved_count": 0,
            "mention_universe_hash": "b" * 64,
            "scope_set_hash": "c" * 64,
        },
        "content_hash": "2" * 64,
        "created_by": "17171717-1717-4717-8717-171717171717",
        "created_at": "2026-09-09T08:00:00Z",
    }

    action_evidence = [fact["actions"][0]["evidence"] for fact in facts]
    name_evidence = {
        "character:linzhou": facts[0]["raw_character_mentions"][0]["evidence"],
        "character:qiao": facts[1]["raw_character_mentions"][1]["evidence"],
        "location:room": facts[0]["location"]["evidence"],
        "location:outside": facts[1]["location"]["evidence"],
        "location:courtyard": facts[2]["location"]["evidence"],
        "prop:box": facts[0]["raw_prop_mentions"][0]["evidence"],
    }
    linzhou_white = "state_character_linzhou_appearance_01_white"
    linzhou_black = "state_character_linzhou_appearance_02_black"
    box_closed = "state_prop_box_01_closed"
    box_open = "state_prop_box_02_open"
    entities = [
        _entity(
            "character:linzhou",
            "character",
            "林舟",
            name_evidence["character:linzhou"],
            [
                _state(
                    linzhou_white,
                    "character_appearance",
                    [SCENE_KEYS[0]],
                    "wardrobe",
                    "白衣",
                    action_evidence[0],
                    next_value=linzhou_black,
                ),
                _state(
                    linzhou_black,
                    "character_appearance",
                    [SCENE_KEYS[1]],
                    "wardrobe",
                    "黑衣",
                    action_evidence[1],
                    previous=linzhou_white,
                ),
            ],
        ),
        _entity(
            "character:qiao",
            "character",
            "乔",
            name_evidence["character:qiao"],
            [
                _state(
                    "state_character_qiao_appearance_01_baseline",
                    "character_appearance",
                    [SCENE_KEYS[1], SCENE_KEYS[2]],
                    "appearance",
                    "乔",
                    name_evidence["character:qiao"],
                )
            ],
        ),
        *[
            _entity(
                identity_key,
                "location",
                identity_names[identity_key][1],
                name_evidence[identity_key],
                [
                    _state(
                        f"state_location_{index + 1:02d}_baseline",
                        "location_state",
                        [SCENE_KEYS[index]],
                        "location",
                        identity_names[identity_key][1],
                        name_evidence[identity_key],
                    )
                ],
            )
            for index, identity_key in enumerate(
                ["location:room", "location:outside", "location:courtyard"]
            )
        ],
        _entity(
            "prop:box",
            "prop",
            "木盒",
            name_evidence["prop:box"],
            [
                _state(
                    box_closed,
                    "prop_state",
                    [SCENE_KEYS[0], SCENE_KEYS[1]],
                    "open_state",
                    "closed",
                    action_evidence[0],
                    next_value=box_open,
                ),
                _state(
                    box_open,
                    "prop_state",
                    [SCENE_KEYS[1], SCENE_KEYS[2]],
                    "open_state",
                    "open",
                    action_evidence[1],
                    previous=box_closed,
                ),
            ],
        ),
    ]
    entities.sort(key=lambda item: item["identity_key"])
    production_candidate = {
        "source_version_id": SOURCE_VERSION_ID,
        "source_hash": _hash(TEXT),
        "structure_identity_set_version_id": STRUCTURE_VERSION_ID,
        "structure_identity_set_version_hash": "2" * 64,
        "scene_fact_candidate_revision_id": SCENE_FACT_REVISION_ID,
        "scene_fact_candidate_revision_hash": "3" * 64,
        "entities": entities,
        "world_claims": [],
        "design_gaps": [],
        "review_issues": [],
    }

    state_by_scene_and_identity = {
        (SCENE_KEYS[0], "location:room"): "state_location_01_baseline",
        (SCENE_KEYS[0], "character:linzhou"): linzhou_white,
        (SCENE_KEYS[0], "prop:box"): box_closed,
        (SCENE_KEYS[1], "location:outside"): "state_location_02_baseline",
        (SCENE_KEYS[1], "character:linzhou"): linzhou_black,
        (SCENE_KEYS[1], "prop:box"): box_open,
        (SCENE_KEYS[1], "character:qiao"): "state_character_qiao_appearance_01_baseline",
        (SCENE_KEYS[2], "location:courtyard"): "state_location_03_baseline",
        (SCENE_KEYS[2], "character:qiao"): "state_character_qiao_appearance_01_baseline",
        (SCENE_KEYS[2], "prop:box"): box_open,
    }
    scenes: list[dict[str, Any]] = []
    occurrence_keys: dict[tuple[int, str], str] = {}
    for index, mappings in enumerate(
        [
            [item for item in mention_mappings if item["temporary_scene_id"] == f"scene_{n:04d}"]
            for n in range(1, 4)
        ]
    ):
        ordered = sorted(mappings, key=lambda item: item["source_start"])
        occurrences: list[dict[str, Any]] = []
        for order, mapping in enumerate(ordered, start=1):
            key = f"occurrence_scene_{index + 1:04d}_{order:04d}"
            occurrence_keys[(index, mapping["identity_key"])] = key
            occurrences.append(
                {
                    "occurrence_key": key,
                    "order": order,
                    "subject_kind": mapping["kind"],
                    "identity_key": mapping["identity_key"],
                    "state_key": state_by_scene_and_identity[
                        (SCENE_KEYS[index], mapping["identity_key"])
                    ],
                    "occurrence_role": "actual",
                    "evidence": {
                        name: mapping[name]
                        for name in ("source_start", "source_end", "text_hash", "exact_anchor")
                    },
                }
            )
        scenes.append(
            {
                "scene_scope_key": SCENE_KEYS[index],
                "scene_owner_logical_id": SCENE_KEYS[index].removeprefix("scene:"),
                "temporary_scene_id": f"scene_{index + 1:04d}",
                "source_start": scene_starts[index],
                "source_end": scene_ends[index],
                "dialogues": [],
                "beats": [
                    {
                        "beat_key": f"beat_scene_{index + 1:04d}_0001",
                        "order": 1,
                        "text": actions[index],
                        "evidence": action_evidence[index],
                    }
                ],
                "occurrences": occurrences,
            }
        )
    scene_binding_candidate = {
        "source_version_id": SOURCE_VERSION_ID,
        "source_hash": _hash(TEXT),
        "structure_identity_set_version_id": STRUCTURE_VERSION_ID,
        "structure_identity_set_version_hash": "2" * 64,
        "scene_fact_candidate_revision_id": SCENE_FACT_REVISION_ID,
        "scene_fact_candidate_revision_hash": "3" * 64,
        "production_entity_candidate_revision_id": PRODUCTION_REVISION_ID,
        "production_entity_candidate_revision_hash": "4" * 64,
        "scenes": scenes,
        "review_issues": [],
    }

    stage_input = InteractionContinuityInput.model_validate(
        {
            "source_version_id": SOURCE_VERSION_ID,
            "source_hash": _hash(TEXT),
            "normalized_text": TEXT,
            "structure_identity_set_version_id": STRUCTURE_VERSION_ID,
            "structure_identity_set_version_hash": "2" * 64,
            "structure_identity_set": structure_identity_set,
            "scene_fact_candidate_revision_id": SCENE_FACT_REVISION_ID,
            "scene_fact_candidate_revision_hash": "3" * 64,
            "scene_fact_candidate": scene_fact_candidate,
            "production_entity_candidate_revision_id": PRODUCTION_REVISION_ID,
            "production_entity_candidate_revision_hash": "4" * 64,
            "production_entity_candidate": production_candidate,
            "scene_binding_candidate_revision_id": SCENE_BINDING_REVISION_ID,
            "scene_binding_candidate_revision_hash": "5" * 64,
            "scene_binding_candidate": scene_binding_candidate,
        }
    )

    geometry_evidence = {
        "hand": action_evidence[0],
        "grip_type": action_evidence[0],
        "contact_point": action_evidence[0],
        "direction": None,
        "relative_scale": None,
    }
    interactions = [
        {
            "interaction_key": "interaction_box_hold",
            "claim_series_key": "interaction_series_box_hold",
            "claim_revision": 1,
            "supersedes_interaction_key": None,
            "scene_scope_key": SCENE_KEYS[0],
            "beat_key": "beat_scene_0001_0001",
            "story_time_key": "storytime:00000001",
            "predicate": "hold",
            "actor_occurrence_key": occurrence_keys[(0, "character:linzhou")],
            "prop_occurrence_key": occurrence_keys[(0, "prop:box")],
            "counterparty_occurrence_key": None,
            "holder_before_identity_key": None,
            "holder_after_identity_key": "character:linzhou",
            "prop_state_before_key": box_closed,
            "prop_state_after_key": box_closed,
            "state_delta": None,
            "hand": "left",
            "grip_type": "power_grip",
            "contact_point": "left_hand",
            "direction": None,
            "relative_scale": None,
            "geometry_evidence": geometry_evidence,
            "evidence": action_evidence[0],
        },
        {
            "interaction_key": "interaction_box_transfer",
            "claim_series_key": "interaction_series_box_transfer",
            "claim_revision": 1,
            "supersedes_interaction_key": None,
            "scene_scope_key": SCENE_KEYS[1],
            "beat_key": "beat_scene_0002_0001",
            "story_time_key": "storytime:00000002",
            "predicate": "give",
            "actor_occurrence_key": occurrence_keys[(1, "character:linzhou")],
            "prop_occurrence_key": occurrence_keys[(1, "prop:box")],
            "counterparty_occurrence_key": occurrence_keys[(1, "character:qiao")],
            "holder_before_identity_key": "character:linzhou",
            "holder_after_identity_key": "character:qiao",
            "prop_state_before_key": box_closed,
            "prop_state_after_key": box_open,
            "state_delta": "closed_to_open",
            "hand": "left",
            "grip_type": "transfer_grip",
            "contact_point": "left_hand",
            "direction": "toward_counterparty",
            "relative_scale": {"numerator": 1, "denominator": 3},
            "geometry_evidence": {
                "hand": action_evidence[1],
                "grip_type": action_evidence[1],
                "contact_point": action_evidence[1],
                "direction": action_evidence[1],
                "relative_scale": action_evidence[1],
            },
            "evidence": action_evidence[1],
        },
        {
            "interaction_key": "interaction_box_carry",
            "claim_series_key": "interaction_series_box_carry",
            "claim_revision": 1,
            "supersedes_interaction_key": None,
            "scene_scope_key": SCENE_KEYS[2],
            "beat_key": "beat_scene_0003_0001",
            "story_time_key": "storytime:00000003",
            "predicate": "carry",
            "actor_occurrence_key": occurrence_keys[(2, "character:qiao")],
            "prop_occurrence_key": occurrence_keys[(2, "prop:box")],
            "counterparty_occurrence_key": None,
            "holder_before_identity_key": "character:qiao",
            "holder_after_identity_key": "character:qiao",
            "prop_state_before_key": box_open,
            "prop_state_after_key": box_open,
            "state_delta": None,
            "hand": "right",
            "grip_type": "cradle_grip",
            "contact_point": "right_hand",
            "direction": "toward_courtyard",
            "relative_scale": None,
            "geometry_evidence": {
                "hand": action_evidence[2],
                "grip_type": action_evidence[2],
                "contact_point": action_evidence[2],
                "direction": action_evidence[2],
                "relative_scale": None,
            },
            "evidence": action_evidence[2],
        },
    ]
    ledger_entries = [
        {
            "ledger_key": "ledger_character_linzhou_scene_0001",
            "subject_kind": "character",
            "identity_key": "character:linzhou",
            "scene_scope_key": SCENE_KEYS[0],
            "story_time_key": "storytime:00000001",
            "state_key": linzhou_white,
            "holder_identity_key": None,
            "location_identity_key": "location:room",
            "transition_interaction_key": None,
            "evidence": [
                name_evidence["character:linzhou"],
                name_evidence["location:room"],
            ],
        },
        {
            "ledger_key": "ledger_prop_box_scene_0001",
            "subject_kind": "prop",
            "identity_key": "prop:box",
            "scene_scope_key": SCENE_KEYS[0],
            "story_time_key": "storytime:00000001",
            "state_key": box_closed,
            "holder_identity_key": "character:linzhou",
            "location_identity_key": "location:room",
            "transition_interaction_key": "interaction_box_hold",
            "evidence": [name_evidence["prop:box"], name_evidence["location:room"]],
        },
        {
            "ledger_key": "ledger_character_linzhou_scene_0002",
            "subject_kind": "character",
            "identity_key": "character:linzhou",
            "scene_scope_key": SCENE_KEYS[1],
            "story_time_key": "storytime:00000002",
            "state_key": linzhou_black,
            "holder_identity_key": None,
            "location_identity_key": "location:outside",
            "transition_interaction_key": None,
            "evidence": [
                facts[1]["raw_character_mentions"][0]["evidence"],
                name_evidence["location:outside"],
            ],
        },
        {
            "ledger_key": "ledger_character_qiao_scene_0002",
            "subject_kind": "character",
            "identity_key": "character:qiao",
            "scene_scope_key": SCENE_KEYS[1],
            "story_time_key": "storytime:00000002",
            "state_key": "state_character_qiao_appearance_01_baseline",
            "holder_identity_key": None,
            "location_identity_key": "location:outside",
            "transition_interaction_key": None,
            "evidence": [name_evidence["character:qiao"], name_evidence["location:outside"]],
        },
        {
            "ledger_key": "ledger_prop_box_scene_0002",
            "subject_kind": "prop",
            "identity_key": "prop:box",
            "scene_scope_key": SCENE_KEYS[1],
            "story_time_key": "storytime:00000002",
            "state_key": box_open,
            "holder_identity_key": "character:qiao",
            "location_identity_key": "location:outside",
            "transition_interaction_key": "interaction_box_transfer",
            "evidence": [
                facts[1]["raw_prop_mentions"][0]["evidence"],
                name_evidence["location:outside"],
            ],
        },
        {
            "ledger_key": "ledger_character_qiao_scene_0003",
            "subject_kind": "character",
            "identity_key": "character:qiao",
            "scene_scope_key": SCENE_KEYS[2],
            "story_time_key": "storytime:00000003",
            "state_key": "state_character_qiao_appearance_01_baseline",
            "holder_identity_key": None,
            "location_identity_key": "location:courtyard",
            "transition_interaction_key": None,
            "evidence": [
                facts[2]["raw_character_mentions"][0]["evidence"],
                name_evidence["location:courtyard"],
            ],
        },
        {
            "ledger_key": "ledger_prop_box_scene_0003",
            "subject_kind": "prop",
            "identity_key": "prop:box",
            "scene_scope_key": SCENE_KEYS[2],
            "story_time_key": "storytime:00000003",
            "state_key": box_open,
            "holder_identity_key": "character:qiao",
            "location_identity_key": "location:courtyard",
            "transition_interaction_key": "interaction_box_carry",
            "evidence": [
                facts[2]["raw_prop_mentions"][0]["evidence"],
                name_evidence["location:courtyard"],
            ],
        },
    ]
    continuity = [
        {
            "continuity_key": "continuity_character_linzhou_appearance_change",
            "claim_series_key": "continuity_series_character_linzhou_appearance",
            "claim_revision": 1,
            "supersedes_continuity_key": None,
            "subject_kind": "character",
            "identity_key": "character:linzhou",
            "from_scene_scope_key": SCENE_KEYS[0],
            "to_scene_scope_key": SCENE_KEYS[1],
            "story_time_start": "storytime:00000001",
            "story_time_end": "storytime:00000002",
            "before_state_key": linzhou_white,
            "after_state_key": linzhou_black,
            "transition": "state_changes",
            "delta": "wardrobe_white_to_black",
            "evidence": [action_evidence[0], action_evidence[1]],
        },
        {
            "continuity_key": "continuity_character_qiao_appearance_persists",
            "claim_series_key": "continuity_series_character_qiao_appearance",
            "claim_revision": 1,
            "supersedes_continuity_key": None,
            "subject_kind": "character",
            "identity_key": "character:qiao",
            "from_scene_scope_key": SCENE_KEYS[1],
            "to_scene_scope_key": SCENE_KEYS[2],
            "story_time_start": "storytime:00000002",
            "story_time_end": "storytime:00000003",
            "before_state_key": "state_character_qiao_appearance_01_baseline",
            "after_state_key": "state_character_qiao_appearance_01_baseline",
            "transition": "state_persists",
            "delta": None,
            "evidence": [
                name_evidence["character:qiao"],
                facts[2]["raw_character_mentions"][0]["evidence"],
            ],
        },
        {
            "continuity_key": "continuity_prop_box_open_state",
            "claim_series_key": "continuity_series_prop_box_open_state",
            "claim_revision": 1,
            "supersedes_continuity_key": None,
            "subject_kind": "prop",
            "identity_key": "prop:box",
            "from_scene_scope_key": SCENE_KEYS[1],
            "to_scene_scope_key": SCENE_KEYS[2],
            "story_time_start": "storytime:00000002",
            "story_time_end": "storytime:00000003",
            "before_state_key": box_open,
            "after_state_key": box_open,
            "transition": "state_persists",
            "delta": None,
            "evidence": [action_evidence[1], action_evidence[2]],
        },
    ]
    candidate = {
        "source_version_id": SOURCE_VERSION_ID,
        "source_hash": _hash(TEXT),
        "structure_identity_set_version_id": STRUCTURE_VERSION_ID,
        "structure_identity_set_version_hash": "2" * 64,
        "scene_fact_candidate_revision_id": SCENE_FACT_REVISION_ID,
        "scene_fact_candidate_revision_hash": "3" * 64,
        "production_entity_candidate_revision_id": PRODUCTION_REVISION_ID,
        "production_entity_candidate_revision_hash": "4" * 64,
        "scene_binding_candidate_revision_id": SCENE_BINDING_REVISION_ID,
        "scene_binding_candidate_revision_hash": "5" * 64,
        "scene_story_times": [
            {"scene_scope_key": key, "story_time_key": f"storytime:{index + 1:08d}"}
            for index, key in enumerate(SCENE_KEYS)
        ],
        "interactions": interactions,
        "continuity_ledger": ledger_entries,
        "continuity": continuity,
        "review_issues": [],
    }
    return stage_input, candidate

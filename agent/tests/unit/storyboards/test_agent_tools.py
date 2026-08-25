from uuid import UUID

from uuid6 import uuid7

from app.modules.storyboards import (
    StoryboardDraftAsset,
    StoryboardDraftInput,
    StoryboardDraftUnit,
)
from app.modules.storyboards.agents import (
    ReviewIssue,
    SceneDraft,
    annotate_storyboard_issues,
    assemble_storyboard,
    build_scene_contexts,
    validate_scene_draft,
)
from app.modules.storyboards.agents.tools import bind_explicit_asset_mentions
from app.modules.storyboards.drafts.provider_schema import StoryboardProviderResult


def _input(
    *,
    target_duration_ms: int = 8_000,
    include_mentioned_asset: bool = False,
) -> StoryboardDraftInput:
    first_scene = uuid7()
    second_scene = uuid7()
    first_heading = uuid7()
    first_dialogue = uuid7()
    second_heading = uuid7()
    second_action = uuid7()
    return StoryboardDraftInput(
        batch_id=uuid7(),
        task_id=uuid7(),
        input_hash="a" * 64,
        script_version_id=uuid7(),
        target_duration_ms=target_duration_ms,
        aspect_ratio="9:16",
        visual_style="冷色现实主义",
        units=(
            StoryboardDraftUnit(
                unit_version_id=first_heading,
                position=1,
                kind="scene_heading",
                exact_text="内景·泵站·夜",
                required_for_coverage=True,
                source_scene_id=first_scene,
                source_dialogue_id=None,
            ),
            StoryboardDraftUnit(
                unit_version_id=first_dialogue,
                position=2,
                kind="dialogue",
                exact_text="沈岚：快走！",
                required_for_coverage=True,
                source_scene_id=first_scene,
                source_dialogue_id=uuid7(),
            ),
            StoryboardDraftUnit(
                unit_version_id=second_heading,
                position=3,
                kind="scene_heading",
                exact_text="外景·堤坝·夜",
                required_for_coverage=True,
                source_scene_id=second_scene,
                source_dialogue_id=None,
            ),
            StoryboardDraftUnit(
                unit_version_id=second_action,
                position=4,
                kind="action",
                exact_text="洪水越过堤坝。",
                required_for_coverage=True,
                source_scene_id=second_scene,
                source_dialogue_id=None,
            ),
        ),
        assets=(
            (
                StoryboardDraftAsset(
                    asset_version_id=uuid7(),
                    position=1,
                    kind="location",
                    name="泵站",
                    state_label="夜间运行中",
                    unit_version_ids=(first_heading, first_dialogue),
                ),
            )
            if include_mentioned_asset
            else ()
        ),
    )


def _result(
    *,
    positions: list[int],
    duration_ms: int,
    proposal_key: str = "main",
    placement: str = "画面左侧",
    continuity_note: str = "保持人物位置与运动方向",
    asset_position: int | None = None,
) -> StoryboardProviderResult:
    dialogue_positions = [position for position in positions if position == 2]
    return StoryboardProviderResult.model_validate(
        {
            "shots": [
                {
                    "proposal_key": proposal_key,
                    "position": 1,
                    "title": "关键动作",
                    "unit_positions": positions,
                    "dialogue_unit_positions": dialogue_positions,
                    "purpose": "呈现关键剧情信息",
                    "continuity_note": continuity_note,
                    "shot_size": "wide",
                    "camera_angle": "eye_level",
                    "camera_movement": "static",
                    "composition": "人物与环境同框",
                    "environment": "夜间场景",
                    "subject_placements": [{"subject_key": "shen-lan", "placement": placement}],
                    "mood_lighting": "冷色侧光",
                    "action_beats": [
                        {"beat_key": "act", "order": 1, "description": "完成关键动作"}
                    ],
                    "duration_ms": duration_ms,
                    "ambient": "水声",
                    "asset_bindings": (
                        []
                        if asset_position is None
                        else [
                            {
                                "asset_position": asset_position,
                                "role": "prop",
                                "subject_key": "switch",
                            }
                        ]
                    ),
                    "first_frame": "人物停在动作开始位置",
                    "keyframe_notes": "动作中点保留环境关系",
                    "risk_codes": [],
                }
            ]
        }
    )


def test_context_builder_groups_fixed_input_by_scene_and_allocates_duration() -> None:
    contexts = build_scene_contexts(_input())

    assert [context.scene_key for context in contexts] == [1, 2]
    assert [[unit.position for unit in context.units] for context in contexts] == [
        [1, 2],
        [3, 4],
    ]
    assert sum(context.target_duration_ms for context in contexts) == 8_000
    assert all(isinstance(context.scene_id, UUID) for context in contexts)


def test_context_builder_scopes_occurrence_bound_assets_to_their_scene() -> None:
    contexts = build_scene_contexts(_input(include_mentioned_asset=True))

    assert [asset.name for asset in contexts[0].assets] == ["泵站"]
    assert contexts[1].assets == ()


def test_hard_gates_report_reference_coverage_duration_and_continuity_issues() -> None:
    context = build_scene_contexts(_input())[0]
    missing_coverage = _result(positions=[1], duration_ms=1_000)

    issues = validate_scene_draft(context, missing_coverage)

    assert {issue.code for issue in issues} == {
        "coverage.required_missing",
        "duration.scene_out_of_range",
    }

    discontinuous = StoryboardProviderResult.model_validate(
        {
            "shots": [
                _result(
                    positions=[1],
                    duration_ms=2_000,
                    proposal_key="left",
                    placement="画面左侧",
                )
                .shots[0]
                .model_dump(mode="json"),
                {
                    **_result(
                        positions=[2],
                        duration_ms=2_000,
                        proposal_key="right",
                        placement="画面右侧",
                        continuity_note="警报声继续",
                    )
                    .shots[0]
                    .model_dump(mode="json"),
                    "position": 2,
                },
            ]
        }
    )

    assert "continuity.side_jump" in {
        issue.code for issue in validate_scene_draft(context, discontinuous)
    }

    stable_anchor_with_opposing_motion = StoryboardProviderResult.model_validate(
        {
            "shots": [
                _result(
                    positions=[1],
                    duration_ms=2_000,
                    proposal_key="right-look-left",
                    placement="画面右侧，视线朝向左侧",
                )
                .shots[0]
                .model_dump(mode="json"),
                {
                    **_result(
                        positions=[2],
                        duration_ms=2_000,
                        proposal_key="right-throw-left",
                        placement="画面右侧，由右向左抛出",
                    )
                    .shots[0]
                    .model_dump(mode="json"),
                    "position": 2,
                },
            ]
        }
    )
    assert "continuity.side_jump" not in {
        issue.code for issue in validate_scene_draft(context, stable_anchor_with_opposing_motion)
    }

    unknown_asset = _result(positions=[1, 2], duration_ms=4_000, asset_position=99)
    assert "asset.unknown_position" in {
        issue.code for issue in validate_scene_draft(context, unknown_asset)
    }


def test_hard_gate_requires_an_explicitly_mentioned_asset_to_be_bound() -> None:
    context = build_scene_contexts(_input(include_mentioned_asset=True))[0]

    unbound = _result(positions=[1, 2], duration_ms=4_000)
    assert "asset.mentioned_unbound" in {
        issue.code for issue in validate_scene_draft(context, unbound)
    }

    bound = _result(positions=[1, 2], duration_ms=4_000, asset_position=1)
    assert "asset.mentioned_unbound" not in {
        issue.code for issue in validate_scene_draft(context, bound)
    }


def test_explicit_asset_mentions_are_bound_deterministically_and_idempotently() -> None:
    context = build_scene_contexts(_input(include_mentioned_asset=True))[0]
    unbound = _result(positions=[1, 2], duration_ms=4_000)

    bound = bind_explicit_asset_mentions(context, unbound)
    rebound = bind_explicit_asset_mentions(context, bound)

    assert [(item.asset_position, item.role) for item in bound.shots[0].asset_bindings] == [
        (1, "location")
    ]
    assert rebound == bound
    assert "asset.mentioned_unbound" not in {
        issue.code for issue in validate_scene_draft(context, bound)
    }


def test_assembler_renumbers_scene_shots_and_calculates_timecodes() -> None:
    contexts = build_scene_contexts(_input())
    assembled = assemble_storyboard(
        contexts,
        (
            SceneDraft(scene_key=1, result=_result(positions=[1, 2], duration_ms=4_000)),
            SceneDraft(scene_key=2, result=_result(positions=[3, 4], duration_ms=4_000)),
        ),
    )

    assert [shot.position for shot in assembled.candidate.shots] == [1, 2]
    assert [shot.proposal_key for shot in assembled.candidate.shots] == [
        "scene-1-main",
        "scene-2-main",
    ]
    assert [row.timecode_in_ms for row in assembled.timeline] == [0, 4_000]
    assert [row.timecode_out_ms for row in assembled.timeline] == [4_000, 8_000]
    assert len(assembled.result_hash) == 64


def test_review_warning_risk_codes_take_priority_over_existing_risk_limit() -> None:
    contexts = build_scene_contexts(_input())
    existing_codes = [f"existing-{position}" for position in range(20)]
    first_result = _result(positions=[1, 2], duration_ms=4_000)
    first_shot = first_result.shots[0].model_copy(update={"risk_codes": existing_codes})
    assembled = assemble_storyboard(
        contexts,
        (
            SceneDraft(
                scene_key=1,
                result=StoryboardProviderResult(shots=[first_shot]),
            ),
            SceneDraft(scene_key=2, result=_result(positions=[3, 4], duration_ms=4_000)),
        ),
    )
    warning = ReviewIssue(
        issue_id="review-priority",
        code="review.priority",
        severity="warning",
        scope="shot",
        scene_key=1,
        shot_positions=(1,),
        evidence="需要人工优先复核",
        repair_hint=None,
        source="reviewer",
    )

    annotated = annotate_storyboard_issues(assembled, (warning, warning))

    assert annotated.candidate.shots[0].risk_codes == [
        "review.priority",
        *existing_codes[:19],
    ]
    assert annotated.timeline[0].shot.risk_codes == annotated.candidate.shots[0].risk_codes

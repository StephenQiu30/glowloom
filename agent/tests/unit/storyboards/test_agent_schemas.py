from app.modules.storyboards.agents import (
    ReviewIssue,
    SceneAnalysis,
    ScenePlan,
    ShotSeed,
    SourceBeat,
)


def test_agent_stage_schemas_are_typed_and_candidate_scoped() -> None:
    analysis = SceneAnalysis(
        scene_key=1,
        beats=(
            SourceBeat(
                beat_key="warning",
                unit_positions=(1, 2),
                dramatic_function="conflict",
                summary="警报打断撤离",
            ),
        ),
        conflict="必须在闸门关闭前撤离",
        reveal=None,
        reaction="沈岚听见警报后转身",
        continuity_facts=("沈岚在画面左侧",),
    )
    plan = ScenePlan(
        scene_key=1,
        spatial_axis="控制台到闸门",
        movement_direction="沈岚由左向右",
        eyeline="沈岚看向画面右侧闸门",
        entrances_exits=("沈岚从左侧进入",),
        prop_states=("控制杆处于抬起状态",),
        rhythm="快速建立后切反应",
        duration_budget_ms=5_000,
        shot_seeds=(
            ShotSeed(
                seed_key="establish",
                beat_keys=("warning",),
                unit_positions=(1, 2),
                purpose="建立警报与撤离方向",
                suggested_duration_ms=5_000,
            ),
        ),
    )
    issue = ReviewIssue(
        issue_id="continuity-1",
        code="continuity.side_jump",
        severity="blocker",
        scope="shot",
        scene_key=1,
        shot_positions=(2,),
        evidence="同一人物从左侧无动作跳到右侧",
        repair_hint="补充越轴动作或保持画面侧别",
        source="reviewer",
    )

    assert analysis.beats[0].unit_positions == (1, 2)
    assert plan.shot_seeds[0].beat_keys == ("warning",)
    assert issue.scene_key == 1
    assert issue.shot_positions == (2,)
    assert issue.model_config.get("frozen") is True

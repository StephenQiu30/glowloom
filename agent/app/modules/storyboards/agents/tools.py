from collections.abc import Mapping, Sequence
from hashlib import sha256
from typing import Literal
from uuid import UUID

from app.modules.storyboards.contracts import StoryboardDraftInput
from app.modules.storyboards.drafts.provider_schema import (
    ProviderAssetBinding,
    ProviderShot,
    StoryboardProviderResult,
)

from .schemas import (
    AssembledStoryboard,
    ReviewIssue,
    SceneContext,
    SceneContextAsset,
    SceneContextUnit,
    SceneDraft,
    StoryboardTimelineShot,
)

DURATION_TOLERANCE = 0.25


def bind_explicit_asset_mentions(
    context: SceneContext,
    result: StoryboardProviderResult,
) -> StoryboardProviderResult:
    """Deterministically bind fixed assets explicitly named by covered source units."""
    role_by_kind = {
        "character": "character",
        "location": "location",
        "setting": "location",
        "prop": "prop",
        "facility": "prop",
        "equipment": "prop",
        "costume": "costume",
        "visual_style": "visual_style",
        "voice": "voice",
    }
    shots = list(result.shots)
    for asset in context.assets:
        mentioned_positions = {
            unit.position
            for unit in context.units
            if asset.name.casefold() in unit.exact_text.casefold()
        }
        if not mentioned_positions or any(
            bool(mentioned_positions.intersection(shot.unit_positions))
            and any(binding.asset_position == asset.position for binding in shot.asset_bindings)
            for shot in shots
        ):
            continue
        target_index = next(
            (
                index
                for index, shot in enumerate(shots)
                if mentioned_positions.intersection(shot.unit_positions)
            ),
            None,
        )
        if target_index is None:
            continue
        role = role_by_kind.get(asset.kind, "prop")
        binding = ProviderAssetBinding.model_validate(
            {
                "asset_position": asset.position,
                "role": role,
                "subject_key": None,
            }
        )
        target = shots[target_index]
        shots[target_index] = target.model_copy(
            update={"asset_bindings": [*target.asset_bindings, binding]}
        )
    return result.model_copy(update={"shots": shots})


def build_scene_contexts(value: StoryboardDraftInput) -> tuple[SceneContext, ...]:
    if not value.units:
        raise ValueError("storyboard harness input requires narrative units")

    grouped: dict[UUID, list[SceneContextUnit]] = {}
    scene_order: list[UUID] = []
    seen_positions: set[int] = set()
    for unit in value.units:
        if unit.source_scene_id is None:
            raise ValueError("every narrative unit must belong to a confirmed scene")
        if unit.position in seen_positions:
            raise ValueError("narrative unit positions must be unique across the input")
        seen_positions.add(unit.position)
        if unit.source_scene_id not in grouped:
            grouped[unit.source_scene_id] = []
            scene_order.append(unit.source_scene_id)
        grouped[unit.source_scene_id].append(
            SceneContextUnit(
                unit_version_id=unit.unit_version_id,
                position=unit.position,
                kind=unit.kind,
                exact_text=unit.exact_text,
                required_for_coverage=unit.required_for_coverage,
                source_dialogue_id=unit.source_dialogue_id,
            )
        )

    minimum_duration = len(scene_order) * 500
    if value.target_duration_ms < minimum_duration:
        raise ValueError("target duration cannot allocate the minimum duration to every scene")

    weights = [
        max(1, sum(len(unit.exact_text) for unit in grouped[scene_id])) for scene_id in scene_order
    ]
    durations = _allocate_duration(value.target_duration_ms, weights)
    world_facts = tuple(fact for entry in value.world_entries for fact in entry.facts)
    world_rules = tuple(rule for entry in value.world_entries for rule in entry.rules)
    return tuple(
        SceneContext(
            scene_key=index,
            scene_id=scene_id,
            target_duration_ms=durations[index - 1],
            aspect_ratio=value.aspect_ratio,
            visual_style=value.visual_style,
            units=tuple(grouped[scene_id]),
            assets=tuple(
                SceneContextAsset(
                    asset_version_id=asset.asset_version_id,
                    position=asset.position,
                    kind=asset.kind,
                    name=asset.name,
                    state_label=asset.state_label,
                )
                for asset in value.assets
                if not asset.unit_version_ids
                or bool(
                    set(asset.unit_version_ids).intersection(
                        unit.unit_version_id for unit in grouped[scene_id]
                    )
                )
            ),
            world_facts=world_facts,
            world_rules=world_rules,
        )
        for index, scene_id in enumerate(scene_order, start=1)
    )


def _allocate_duration(total_ms: int, weights: Sequence[int]) -> list[int]:
    remaining = total_ms
    remaining_weight = sum(weights)
    durations: list[int] = []
    for index, weight in enumerate(weights):
        remaining_scenes = len(weights) - index - 1
        if remaining_scenes == 0:
            duration = remaining
        else:
            proportional = round(remaining * weight / remaining_weight)
            maximum = remaining - remaining_scenes * 500
            duration = min(max(500, proportional), maximum)
        durations.append(duration)
        remaining -= duration
        remaining_weight -= weight
    return durations


def validate_scene_draft(
    context: SceneContext,
    result: StoryboardProviderResult,
    *,
    unit_scene_keys: Mapping[int, int] | None = None,
) -> tuple[ReviewIssue, ...]:
    scene_positions = {unit.position for unit in context.units}
    dialogue_positions = {
        unit.position for unit in context.units if unit.source_dialogue_id is not None
    }
    asset_positions = {asset.position for asset in context.assets}
    required_positions = {unit.position for unit in context.units if unit.required_for_coverage}
    covered_positions: set[int] = set()
    issues: list[ReviewIssue] = []

    for shot in result.shots:
        for position in shot.unit_positions:
            if position in scene_positions:
                covered_positions.add(position)
                continue
            code = "reference.unknown_position"
            if unit_scene_keys is not None and position in unit_scene_keys:
                code = "reference.cross_scene"
            issues.append(
                _tool_issue(
                    code=code,
                    scene_key=context.scene_key,
                    shot_positions=(shot.position,),
                    evidence=f"镜头引用位置 {position} 不属于当前场景",
                    repair_hint="仅保留当前场景固定输入中的 position",
                )
            )
        invalid_dialogue = set(shot.dialogue_unit_positions).difference(dialogue_positions)
        if invalid_dialogue:
            issues.append(
                _tool_issue(
                    code="reference.dialogue_invalid",
                    scene_key=context.scene_key,
                    shot_positions=(shot.position,),
                    evidence=f"对白位置 {sorted(invalid_dialogue)} 不是当前场景的对白",
                    repair_hint="移除错误对白引用或绑定当前场景的对白单元",
                )
            )
        invalid_assets = {binding.asset_position for binding in shot.asset_bindings}.difference(
            asset_positions
        )
        if invalid_assets:
            issues.append(
                _tool_issue(
                    code="asset.unknown_position",
                    scene_key=context.scene_key,
                    shot_positions=(shot.position,),
                    evidence=f"镜头绑定了未知资产位置：{sorted(invalid_assets)}",
                    repair_hint="仅绑定固定输入中存在的 asset_position",
                )
            )

    missing = sorted(required_positions.difference(covered_positions))
    if missing:
        issues.append(
            _tool_issue(
                code="coverage.required_missing",
                scene_key=context.scene_key,
                evidence=f"必需叙事位置未覆盖：{missing}",
                repair_hint="在有独立叙事贡献的镜头中覆盖这些位置",
            )
        )

    for asset in context.assets:
        mentioned_positions = {
            unit.position
            for unit in context.units
            if asset.name.casefold() in unit.exact_text.casefold()
        }
        if not mentioned_positions:
            continue
        bound_on_source = any(
            bool(mentioned_positions.intersection(shot.unit_positions))
            and any(binding.asset_position == asset.position for binding in shot.asset_bindings)
            for shot in result.shots
        )
        if not bound_on_source:
            issues.append(
                _tool_issue(
                    code="asset.mentioned_unbound",
                    scene_key=context.scene_key,
                    evidence=(
                        f"来源位置 {sorted(mentioned_positions)} 明确提到资产 "
                        f"{asset.name!r}，但覆盖镜头未绑定资产位置 {asset.position}"
                    ),
                    repair_hint="在覆盖该来源位置的镜头中绑定对应 asset_position",
                )
            )

    total_duration_ms = sum(shot.duration_ms for shot in result.shots)
    duration_min = round(context.target_duration_ms * (1 - DURATION_TOLERANCE))
    duration_max = round(context.target_duration_ms * (1 + DURATION_TOLERANCE))
    if not duration_min <= total_duration_ms <= duration_max:
        issues.append(
            _tool_issue(
                code="duration.scene_out_of_range",
                scene_key=context.scene_key,
                evidence=(
                    f"场景镜头时长 {total_duration_ms}ms 不在 "
                    f"{duration_min}ms–{duration_max}ms 范围"
                ),
                repair_hint="调整镜头拆分或单镜时长，保持动作与对白可执行",
            )
        )

    issues.extend(_continuity_issues(context, result))
    return _deduplicate_issues(issues)


def _continuity_issues(
    context: SceneContext,
    result: StoryboardProviderResult,
) -> list[ReviewIssue]:
    issues: list[ReviewIssue] = []
    previous_sides: dict[str, str] = {}
    transition_terms = ("移动", "越轴", "换边", "穿过", "转场", "cross", "move")
    for shot in result.shots:
        current_sides = {
            placement.subject_key: side
            for placement in shot.subject_placements
            if (side := _placement_side(placement.placement)) is not None
        }
        for subject_key, current_side in current_sides.items():
            previous_side = previous_sides.get(subject_key)
            if (
                previous_side is not None
                and {previous_side, current_side} == {"left", "right"}
                and not any(term in shot.continuity_note.lower() for term in transition_terms)
            ):
                issues.append(
                    _tool_issue(
                        code="continuity.side_jump",
                        scene_key=context.scene_key,
                        shot_positions=(shot.position,),
                        evidence=(
                            f"主体 {subject_key} 从 {previous_side} 无过渡跳到 {current_side}"
                        ),
                        repair_hint="保持画面侧别，或在 continuity_note 中给出可见换边动作",
                    )
                )
        previous_sides.update(current_sides)
    return issues


def _placement_side(value: str) -> str | None:
    normalized = value.lower()
    anchors = {
        "left": ("左", "left"),
        "right": ("右", "right"),
        "center": ("中", "center", "centre"),
    }
    matches = [
        (index, side)
        for side, terms in anchors.items()
        for term in terms
        if (index := normalized.find(term)) >= 0
    ]
    return min(matches)[1] if matches else None


def validate_review_scope(
    contexts: Sequence[SceneContext],
    drafts: Sequence[SceneDraft],
    issues: Sequence[ReviewIssue],
) -> tuple[ReviewIssue, ...]:
    context_keys = {context.scene_key for context in contexts}
    shot_positions = {
        draft.scene_key: {shot.position for shot in draft.result.shots} for draft in drafts
    }
    invalid: list[ReviewIssue] = []
    for issue in issues:
        if issue.scene_key is not None and issue.scene_key not in context_keys:
            invalid.append(
                _tool_issue(
                    code="review.scope_invalid",
                    evidence=f"审核问题 {issue.issue_id} 引用了未知场景",
                    repair_hint="审核问题必须引用输入中的 scene_key",
                    scope="global",
                )
            )
        elif issue.scope == "shot" and not set(issue.shot_positions).issubset(
            shot_positions.get(issue.scene_key or 0, set())
        ):
            invalid.append(
                _tool_issue(
                    code="review.scope_invalid",
                    evidence=f"审核问题 {issue.issue_id} 引用了未知镜头",
                    repair_hint="审核问题必须引用对应场景中的局部镜号",
                    scope="global",
                )
            )
    return _deduplicate_issues(invalid)


def enforce_review_policy(
    contexts: Sequence[SceneContext],
    drafts: Sequence[SceneDraft],
    issues: Sequence[ReviewIssue],
) -> tuple[ReviewIssue, ...]:
    """Apply deterministic scope and severity policy to model-authored review issues."""
    context_keys = {context.scene_key for context in contexts}
    shot_positions = {
        draft.scene_key: {shot.position for shot in draft.result.shots} for draft in drafts
    }
    accepted: list[ReviewIssue] = []
    for issue in issues:
        issue = issue.model_copy(update={"source": "reviewer"})
        if issue.scene_key is not None and issue.scene_key not in context_keys:
            accepted.append(
                _tool_issue(
                    code="review.scope_invalid",
                    evidence=f"审核问题 {issue.issue_id} 引用了未知场景",
                    repair_hint="审核问题必须引用输入中的 scene_key",
                    scope="global",
                ).model_copy(update={"severity": "warning"})
            )
            continue
        if issue.scope == "shot" and not set(issue.shot_positions).issubset(
            shot_positions.get(issue.scene_key or 0, set())
        ):
            accepted.append(
                _tool_issue(
                    code="review.scope_invalid",
                    evidence=f"审核问题 {issue.issue_id} 引用了未知镜头",
                    repair_hint="审核问题必须引用对应场景中的局部镜号",
                    scope="global",
                ).model_copy(update={"severity": "warning"})
            )
            continue

        if issue.severity == "blocker" and "asset" in issue.code.casefold():
            accepted.append(
                issue.model_copy(
                    update={
                        "severity": "warning",
                        "repair_hint": (
                            "资产引用完整性由确定性工具校验；Reviewer 的语义判断"
                            "保留为人工审核提示，不触发自动修复。"
                        ),
                    }
                )
            )
            continue
        accepted.append(issue)
    return _deduplicate_issues(accepted)


def assemble_storyboard(
    contexts: Sequence[SceneContext],
    drafts: Sequence[SceneDraft],
) -> AssembledStoryboard:
    drafts_by_scene = {draft.scene_key: draft for draft in drafts}
    if len(drafts_by_scene) != len(drafts):
        raise ValueError("scene drafts must have unique scene keys")
    if set(drafts_by_scene) != {context.scene_key for context in contexts}:
        raise ValueError("every scene context requires exactly one scene draft")

    global_shots: list[ProviderShot] = []
    timeline: list[StoryboardTimelineShot] = []
    timecode_ms = 0
    for context in contexts:
        draft = drafts_by_scene[context.scene_key]
        for shot in draft.result.shots:
            global_position = len(global_shots) + 1
            global_shot = shot.model_copy(
                update={
                    "proposal_key": f"scene-{context.scene_key}-{shot.proposal_key}",
                    "position": global_position,
                }
            )
            global_shots.append(global_shot)
            timeline.append(
                StoryboardTimelineShot(
                    scene_key=context.scene_key,
                    scene_id=context.scene_id,
                    local_position=shot.position,
                    global_position=global_position,
                    timecode_in_ms=timecode_ms,
                    timecode_out_ms=timecode_ms + shot.duration_ms,
                    original_proposal_key=shot.proposal_key,
                    shot=global_shot,
                )
            )
            timecode_ms += shot.duration_ms

    candidate = StoryboardProviderResult(shots=global_shots)
    result_hash = sha256(candidate.model_dump_json().encode("utf-8")).hexdigest()
    return AssembledStoryboard(
        candidate=candidate,
        timeline=tuple(timeline),
        total_duration_ms=timecode_ms,
        result_hash=result_hash,
    )


def annotate_storyboard_issues(
    assembled: AssembledStoryboard,
    issues: Sequence[ReviewIssue],
) -> AssembledStoryboard:
    """Carry scoped non-blocking review evidence into persisted shot risks."""
    timeline: list[StoryboardTimelineShot] = []
    shots: list[ProviderShot] = []
    for row in assembled.timeline:
        scoped_codes = [
            issue.code[:80]
            for issue in issues
            if issue.severity == "warning"
            and (
                issue.scope == "global"
                or (
                    issue.scene_key == row.scene_key
                    and (issue.scope == "scene" or row.local_position in issue.shot_positions)
                )
            )
        ]
        # Review warnings are the latest normalized evidence. Keep their stable
        # order ahead of pre-existing risks so the 20-code schema limit cannot
        # silently discard them; older risks fall off from the tail instead.
        risk_codes = list(dict.fromkeys((*scoped_codes, *row.shot.risk_codes)))[:20]
        shot = row.shot.model_copy(update={"risk_codes": risk_codes})
        shots.append(shot)
        timeline.append(row.model_copy(update={"shot": shot}))

    candidate = StoryboardProviderResult(shots=shots)
    return AssembledStoryboard(
        candidate=candidate,
        timeline=tuple(timeline),
        total_duration_ms=assembled.total_duration_ms,
        result_hash=sha256(candidate.model_dump_json().encode("utf-8")).hexdigest(),
    )


def _tool_issue(
    *,
    code: str,
    evidence: str,
    repair_hint: str,
    scene_key: int | None = None,
    shot_positions: tuple[int, ...] = (),
    scope: Literal["global", "scene", "shot"] | None = None,
) -> ReviewIssue:
    resolved_scope = scope or ("shot" if shot_positions else "scene")
    issue_key = f"{code}:{scene_key}:{','.join(str(value) for value in shot_positions)}"
    return ReviewIssue(
        issue_id=issue_key,
        code=code,
        severity="blocker",
        scope=resolved_scope,
        scene_key=scene_key,
        shot_positions=shot_positions,
        evidence=evidence,
        repair_hint=repair_hint,
        source="tool",
    )


def _deduplicate_issues(issues: Sequence[ReviewIssue]) -> tuple[ReviewIssue, ...]:
    unique: dict[tuple[str, int | None, tuple[int, ...]], ReviewIssue] = {}
    for issue in issues:
        key = (issue.code, issue.scene_key, issue.shot_positions)
        existing = unique.get(key)
        if existing is None or (existing.severity == "warning" and issue.severity == "blocker"):
            unique[key] = issue
    return tuple(unique.values())

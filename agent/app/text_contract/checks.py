from typing import Any

from app.protocol.canonical import canonical_hash
from app.text_contract.schemas import (
    EpisodeAnalysis,
    EpisodeMap,
    Issue,
    Scene,
    SceneDirection,
    WorldBook,
)
from app.text_contract.task import TextResult, TextTask
from app.text_contract.validation import (
    check_direction,
    check_episode,
    check_episode_map,
    check_evidence,
    check_world,
)


def scene_for(task: TextTask) -> Scene:
    for analysis in task.analyses:
        if analysis.episode_key == task.episode_key:
            for scene in analysis.scenes:
                if scene.key == task.scene_key:
                    return scene
    raise ValueError("selected scene is missing")


def validate_result(task: TextTask, raw: dict[str, Any]) -> TextResult:
    result = TextResult.model_validate(raw)
    if (
        result.invocation_id != task.invocation_id
        or result.stage != task.stage
        or result.input_hash != canonical_hash(task.model_dump(mode="json"))
        or result.release_hash != task.release_hash
        or result.context.source_revision_id != task.source.revision_id
        or result.context.source_hash != task.source.content_hash
        or result.candidate_hash != canonical_hash(result.candidate)
    ):
        raise ValueError("result_binding_mismatch")
    if task.stage == "map_manuscript":
        candidate = EpisodeMap.model_validate(result.candidate)
        issues = check_episode_map(task.source, candidate)
    elif task.stage == "analyze_episode" and task.episode_map is not None:
        candidate = EpisodeAnalysis.model_validate(result.candidate)
        episode = next(item for item in task.episode_map.episodes if item.key == task.episode_key)
        issues = check_episode(task.source, episode, candidate)
    elif task.stage == "build_world":
        candidate = WorldBook.model_validate(result.candidate)
        issues = check_world(task.source, task.analyses, candidate)
    elif task.stage == "direct_scene":
        candidate = SceneDirection.model_validate(result.candidate)
        issues = check_direction(task.source, task.episode_key or "", scene_for(task), candidate)
        issues.append(
            Issue(
                code="continuity_mapping_pending",
                scope=task.scene_key or "scene",
                severity="blocker",
                summary="跨场状态与身份披露仍需平台已审阅映射；本结果仅为导演草案。",
            )
        )
    else:
        raise ValueError("task has no valid result checker")
    if result.evidence != check_evidence(task.source, candidate) or result.issues != issues:
        raise ValueError("result_checks_mismatch")
    return result

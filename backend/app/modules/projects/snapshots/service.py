from datetime import UTC, datetime
from typing import Literal
from uuid import UUID

from sqlalchemy.exc import SQLAlchemyError
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.auth import AccessTokenClaims
from app.core.errors import ApiError
from app.modules.assets import ProjectAssetSummary, summarize_project_assets
from app.modules.identity import Capability
from app.modules.production import EpisodeTaskSummary, summarize_episode_tasks
from app.modules.projects import repository
from app.modules.projects.authorization import owned_project
from app.modules.projects.episodes.service import episode_for_read
from app.modules.projects.models import Episode
from app.modules.projects.snapshots.schemas import (
    AssetSummary,
    BlockingReason,
    CostSummary,
    EpisodeProductionSnapshot,
    NextAction,
    PartialFailure,
    ProjectProductionSnapshot,
    ReviewSummary,
    ScriptSummary,
    StoryboardSummary,
    TaskSummary,
)
from app.modules.scripts import ScriptProductionSummary, summarize_current_scripts
from app.modules.storyboards import (
    EpisodeStoryboardSummary,
    summarize_episode_storyboards,
)

_STAGE_ORDER = {
    "script_import": 0,
    "structure_review": 1,
    "asset_preparation": 2,
    "storyboard_preparation": 3,
}


def _unavailable_script(current_version_id: UUID | None) -> ScriptProductionSummary:
    return ScriptProductionSummary(status="unavailable", current_version_id=current_version_id)


def _unavailable_assets() -> ProjectAssetSummary:
    return ProjectAssetSummary(
        status="unavailable",
        total=0,
        versioned=0,
        ready=0,
        draft=0,
        blocked=0,
        ready_kinds=(),
    )


def _unavailable_storyboard() -> EpisodeStoryboardSummary:
    return EpisodeStoryboardSummary(
        status="unavailable",
        total=0,
        ready=0,
        blocked=0,
        unavailable=0,
    )


def _script_response(summary: ScriptProductionSummary) -> ScriptSummary:
    return ScriptSummary(
        status=summary.status,
        current_version_id=summary.current_version_id,
        extraction_batch_id=summary.extraction_batch_id,
        pending_required_candidates=summary.pending_required_candidates,
    )


def _asset_response(summary: ProjectAssetSummary) -> AssetSummary:
    return AssetSummary(
        status=summary.status,
        total=summary.total,
        versioned=summary.versioned,
        ready=summary.ready,
        draft=summary.draft,
        blocked=summary.blocked,
        ready_kinds=list(summary.ready_kinds),
        required_kinds=list(summary.required_kinds),
    )


def _storyboard_response(summary: EpisodeStoryboardSummary) -> StoryboardSummary:
    return StoryboardSummary(
        status=summary.status,
        total=summary.total,
        ready=summary.ready,
        blocked=summary.blocked,
        unavailable=summary.unavailable,
    )


def _task_response(summary: EpisodeTaskSummary | None) -> TaskSummary:
    if summary is None:
        return TaskSummary(status="unavailable")
    if summary.running:
        status = "running"
    elif summary.failed or summary.unknown:
        status = "failed"
    elif summary.succeeded:
        status = "succeeded"
    else:
        status = "not_started"
    return TaskSummary(
        status=status,
        running=summary.running,
        failed=summary.failed,
        succeeded=summary.succeeded,
        unknown=summary.unknown,
    )


def _reason(episode: Episode, code: str, summary: str) -> BlockingReason:
    return BlockingReason(
        code=code,
        summary=summary,
        resource_type="episode",
        resource_id=episode.id,
    )


def _action(episode: Episode, code: str, label: str, panel: str) -> NextAction:
    return NextAction(
        code=code,
        label=label,
        href=f"/studio/{episode.id}/{panel}",
    )


def _episode_snapshot(
    episode: Episode,
    currency: str,
    computed_at: datetime,
    script: ScriptProductionSummary,
    assets: ProjectAssetSummary,
    storyboards: EpisodeStoryboardSummary,
    tasks: EpisodeTaskSummary | None,
    partial_failures: list[PartialFailure],
) -> EpisodeProductionSnapshot:
    review_status: Literal["not_started", "pending", "completed", "unavailable"]
    review_status = "not_started"
    if script.status == "unavailable":
        stage = "script_import"
        completion = 0
        blockers = [_reason(episode, "SCRIPT_UNAVAILABLE", "剧本事实暂时不可用")]
        actions = [_action(episode, "retry_snapshot", "重试读取", "script")]
        review_status = "unavailable"
    elif script.status == "not_started":
        stage = "script_import"
        completion = 0
        blockers = [_reason(episode, "SCRIPT_MISSING", "单集尚未导入剧本")]
        actions = [_action(episode, "import_script", "导入剧本", "script")]
    elif script.status == "published":
        stage = "structure_review"
        completion = 20
        blockers = [_reason(episode, "EXTRACTION_MISSING", "当前剧本尚未提取结构")]
        actions = [_action(episode, "start_extraction", "开始结构提取", "script")]
    elif script.status == "extracting":
        stage = "structure_review"
        completion = 30
        blockers = [_reason(episode, "EXTRACTION_RUNNING", "剧本结构正在提取")]
        actions = [_action(episode, "poll_task", "查看提取任务", "script")]
    elif script.status == "extraction_blocked":
        stage = "structure_review"
        completion = 30
        blockers = [_reason(episode, "EXTRACTION_BLOCKED", "剧本结构提取未完成")]
        actions = [_action(episode, "review_extraction", "查看失败原因", "script")]
    elif script.status == "review_required":
        stage = "structure_review"
        completion = 40
        blockers = [
            _reason(
                episode,
                "CANDIDATES_PENDING",
                f"仍有 {script.pending_required_candidates} 项必需候选待决议",
            )
        ]
        actions = [_action(episode, "review_candidates", "处理候选", "script")]
        review_status = "pending"
    elif script.status == "confirmation_required":
        stage = "structure_review"
        completion = 50
        blockers = [_reason(episode, "STRUCTURE_CONFIRMATION_REQUIRED", "候选已决议，等待确认结构")]
        actions = [_action(episode, "confirm_structure", "确认剧本结构", "script")]
        review_status = "completed"
    elif script.status == "set_current_required":
        stage = "structure_review"
        completion = 55
        blockers = [_reason(episode, "CONFIRMED_SCRIPT_NOT_CURRENT", "已确认结构尚未设为当前剧本")]
        actions = [_action(episode, "set_current_script", "使用确认版本", "script")]
        review_status = "completed"
    elif assets.status == "unavailable":
        stage = "asset_preparation"
        completion = 60
        blockers = [_reason(episode, "ASSETS_UNAVAILABLE", "资产准备度暂时不可用")]
        actions = [_action(episode, "retry_snapshot", "重试读取", "assets")]
        review_status = "completed"
    elif assets.status != "ready":
        stage = "asset_preparation"
        completion = 60
        missing_kinds = sorted(set(assets.required_kinds) - set(assets.ready_kinds))
        if assets.total == 0:
            code, summary = "ASSETS_MISSING", "项目尚未建立角色、场景和声音资产"
        elif assets.blocked:
            code, summary = "ASSETS_BLOCKED", f"有 {assets.blocked} 个资产版本被媒体或授权阻断"
        else:
            code = "ASSET_KINDS_NOT_READY"
            summary = f"仍需就绪资产类型：{', '.join(missing_kinds)}"
        blockers = [_reason(episode, code, summary)]
        actions = [_action(episode, "prepare_assets", "完善资产", "assets")]
        review_status = "completed"
    elif storyboards.status == "unavailable":
        stage = "storyboard_preparation"
        completion = 75
        blockers = [_reason(episode, "STORYBOARD_UNAVAILABLE", "分镜准备度暂时不可用")]
        actions = [_action(episode, "retry_snapshot", "重试读取", "storyboard")]
        review_status = "completed"
    elif storyboards.status == "not_started":
        stage = "storyboard_preparation"
        completion = 75
        blockers = [
            _reason(episode, "STORYBOARD_NOT_STARTED", "角色、场景和声音已就绪，可以开始分镜")
        ]
        actions = [_action(episode, "prepare_storyboard", "开始分镜", "storyboard")]
        review_status = "completed"
    elif storyboards.status == "blocked":
        stage = "storyboard_preparation"
        completion = 80
        blockers = [
            _reason(
                episode,
                "STORYBOARD_BLOCKED",
                f"仍有 {storyboards.blocked} 个分镜未满足生产准备度",
            )
        ]
        actions = [_action(episode, "complete_storyboard", "完善分镜", "storyboard")]
        review_status = "completed"
    else:
        stage = "storyboard_preparation"
        completion = 90
        blockers = []
        actions = [_action(episode, "review_storyboard", "检查分镜", "storyboard")]
        review_status = "completed"

    return EpisodeProductionSnapshot(
        episode_id=episode.id,
        current_stage=stage,
        completion=completion,
        blocking_reasons=blockers,
        next_actions=actions,
        script_summary=_script_response(script),
        asset_summary=_asset_response(assets),
        storyboard_summary=_storyboard_response(storyboards),
        task_summary=_task_response(tasks),
        review_summary=ReviewSummary(
            status=review_status,
            pending=script.pending_required_candidates,
        ),
        cost_summary=CostSummary(currency=currency),
        partial_failures=partial_failures,
        computed_at=computed_at,
    )


async def _compose_snapshots(
    session: AsyncSession,
    episodes: list[Episode],
    *,
    workspace_id: UUID,
    project_id: UUID,
    currency: str,
) -> list[EpisodeProductionSnapshot]:
    computed_at = datetime.now(UTC)
    current_versions = {episode.id: episode.current_script_version_id for episode in episodes}
    partial_failures: list[PartialFailure] = []
    try:
        scripts = await summarize_current_scripts(session, current_versions)
    except (ApiError, SQLAlchemyError):
        scripts = {
            episode_id: _unavailable_script(version_id)
            for episode_id, version_id in current_versions.items()
        }
        partial_failures.append(
            PartialFailure(
                module="scripts",
                code="SCRIPT_SUMMARY_UNAVAILABLE",
                summary="剧本摘要暂时不可用",
            )
        )
    try:
        assets = await summarize_project_assets(
            session,
            workspace_id,
            project_id,
            purpose="ai_short_drama_generation",
            channel="lanverse_preview",
            region="CN",
        )
    except (ApiError, SQLAlchemyError):
        assets = _unavailable_assets()
        partial_failures.append(
            PartialFailure(
                module="assets",
                code="ASSET_SUMMARY_UNAVAILABLE",
                summary="资产摘要暂时不可用",
            )
        )
    try:
        task_summaries: dict[UUID, EpisodeTaskSummary | None] = {
            **await summarize_episode_tasks(
                session, workspace_id, [episode.id for episode in episodes]
            )
        }
    except (ApiError, SQLAlchemyError):
        task_summaries = {episode.id: None for episode in episodes}
        partial_failures.append(
            PartialFailure(
                module="production",
                code="TASK_SUMMARY_UNAVAILABLE",
                summary="任务摘要暂时不可用",
            )
        )
    try:
        storyboard_summaries = await summarize_episode_storyboards(
            session,
            workspace_id,
            project_id,
            [episode.id for episode in episodes],
        )
    except (ApiError, SQLAlchemyError):
        storyboard_summaries = {episode.id: _unavailable_storyboard() for episode in episodes}
        partial_failures.append(
            PartialFailure(
                module="storyboards",
                code="STORYBOARD_SUMMARY_UNAVAILABLE",
                summary="分镜摘要暂时不可用",
            )
        )
    else:
        if any(
            storyboard_summaries.get(episode.id, _unavailable_storyboard()).status == "unavailable"
            for episode in episodes
        ):
            partial_failures.append(
                PartialFailure(
                    module="storyboards",
                    code="STORYBOARD_SUMMARY_UNAVAILABLE",
                    summary="分镜摘要暂时不可用",
                )
            )
    return [
        _episode_snapshot(
            episode,
            currency,
            computed_at,
            scripts.get(episode.id, _unavailable_script(episode.current_script_version_id)),
            assets,
            storyboard_summaries.get(episode.id, _unavailable_storyboard()),
            task_summaries.get(episode.id),
            partial_failures,
        )
        for episode in episodes
    ]


async def episode_production_snapshot(
    session: AsyncSession,
    claims: AccessTokenClaims,
    episode_id: UUID,
) -> EpisodeProductionSnapshot:
    episode, project, _ = await episode_for_read(session, claims, episode_id)
    snapshots = await _compose_snapshots(
        session,
        [episode],
        workspace_id=project.workspace_id,
        project_id=project.id,
        currency=project.currency,
    )
    return snapshots[0]


async def project_production_snapshot(
    session: AsyncSession,
    claims: AccessTokenClaims,
    project_id: UUID,
) -> ProjectProductionSnapshot:
    project, _ = await owned_project(session, claims, project_id, Capability.CONTENT_READ)
    episodes = await repository.list_episodes(session, project_id, include_archived=False)
    computed_at = datetime.now(UTC)
    if not episodes:
        return ProjectProductionSnapshot(
            project_id=project.id,
            current_stage="script_import",
            completion=0,
            blocking_reasons=[
                BlockingReason(
                    code="SCRIPT_DOCUMENT_MISSING",
                    summary="项目尚未导入剧本文档",
                    resource_type="project",
                    resource_id=project.id,
                )
            ],
            next_actions=[
                NextAction(
                    code="import_script_document",
                    label="导入并预览剧本",
                    href=f"/projects/{project.id}#script-import",
                )
            ],
            episodes=[],
            partial_failures=[],
            computed_at=computed_at,
        )
    snapshots = await _compose_snapshots(
        session,
        episodes,
        workspace_id=project.workspace_id,
        project_id=project.id,
        currency=project.currency,
    )
    first = min(snapshots, key=lambda item: _STAGE_ORDER[item.current_stage])
    return ProjectProductionSnapshot(
        project_id=project.id,
        current_stage=first.current_stage,
        completion=min(item.completion for item in snapshots),
        blocking_reasons=[reason for snapshot in snapshots for reason in snapshot.blocking_reasons],
        next_actions=first.next_actions[:1],
        episodes=snapshots,
        partial_failures=list(
            {
                (failure.module, failure.code): failure
                for snapshot in snapshots
                for failure in snapshot.partial_failures
            }.values()
        ),
        computed_at=snapshots[0].computed_at,
    )

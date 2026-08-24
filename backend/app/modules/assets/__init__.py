"""Public asset contracts and version-resolution use cases."""

from app.modules.assets.contracts import (
    AssetCandidateCommand,
    AssetCandidateOccurrence,
    AssetCandidateResult,
    AssetExportSnapshot,
    AssetVersionReadinessReference,
    AssetVersionReference,
    ProjectAssetReferenceSummary,
    ProjectAssetSummary,
    StoryboardAssetInput,
)
from app.modules.assets.models import Asset, AssetNameRevision, AssetState, AssetVersion
from app.modules.assets.repository import (
    find_state_by_key as find_asset_state_by_key,
)
from app.modules.assets.repository import (
    latest_version_number as latest_asset_version_number,
)
from app.modules.assets.schemas import parse_asset_spec, spec_to_json
from app.modules.assets.service import (
    asset_version_exists,
    asset_version_for_content_read,
    create_or_link_candidate,
    link_existing_state_occurrence,
    resolve_asset_version,
    resolve_asset_version_readiness,
    resolve_asset_versions_readiness,
    resolve_episode_storyboard_asset_state_ids,
    resolve_episode_storyboard_asset_units,
    resolve_export_assets,
    resolve_storyboard_assets,
    resolve_storyboard_planning_assets,
    summarize_project_asset_references,
    summarize_project_assets,
)

__all__ = [
    "AssetCandidateCommand",
    "AssetCandidateOccurrence",
    "AssetCandidateResult",
    "AssetExportSnapshot",
    "AssetVersionReadinessReference",
    "AssetVersionReference",
    "Asset",
    "AssetNameRevision",
    "AssetState",
    "AssetVersion",
    "ProjectAssetReferenceSummary",
    "ProjectAssetSummary",
    "StoryboardAssetInput",
    "asset_version_exists",
    "asset_version_for_content_read",
    "create_or_link_candidate",
    "link_existing_state_occurrence",
    "find_asset_state_by_key",
    "latest_asset_version_number",
    "parse_asset_spec",
    "resolve_asset_version",
    "resolve_asset_version_readiness",
    "resolve_asset_versions_readiness",
    "resolve_export_assets",
    "resolve_episode_storyboard_asset_state_ids",
    "resolve_episode_storyboard_asset_units",
    "resolve_storyboard_assets",
    "resolve_storyboard_planning_assets",
    "summarize_project_asset_references",
    "summarize_project_assets",
    "spec_to_json",
]

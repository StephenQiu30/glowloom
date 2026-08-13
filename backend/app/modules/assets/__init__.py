"""Public asset contracts and version-resolution use cases."""

from app.modules.assets.contracts import (
    AssetCandidateCommand,
    AssetCandidateResult,
    AssetVersionReadinessReference,
    AssetVersionReference,
    ProjectAssetReferenceSummary,
    ProjectAssetSummary,
    StoryboardAssetInput,
)
from app.modules.assets.service import (
    asset_version_exists,
    asset_version_for_content_read,
    create_or_link_candidate,
    resolve_asset_version,
    resolve_asset_version_readiness,
    resolve_asset_versions_readiness,
    resolve_storyboard_assets,
    summarize_project_asset_references,
    summarize_project_assets,
)

__all__ = [
    "AssetCandidateCommand",
    "AssetCandidateResult",
    "AssetVersionReadinessReference",
    "AssetVersionReference",
    "ProjectAssetReferenceSummary",
    "ProjectAssetSummary",
    "StoryboardAssetInput",
    "asset_version_exists",
    "asset_version_for_content_read",
    "create_or_link_candidate",
    "resolve_asset_version",
    "resolve_asset_version_readiness",
    "resolve_asset_versions_readiness",
    "resolve_storyboard_assets",
    "summarize_project_asset_references",
    "summarize_project_assets",
]

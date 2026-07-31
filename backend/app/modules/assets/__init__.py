"""Public asset contracts and version-resolution use cases."""

from app.modules.assets.contracts import (
    AssetCandidateCommand,
    AssetCandidateResult,
    AssetVersionReadinessReference,
    AssetVersionReference,
    ProjectAssetSummary,
)
from app.modules.assets.service import (
    asset_version_exists,
    create_or_link_candidate,
    resolve_asset_version,
    resolve_asset_version_readiness,
    summarize_project_assets,
)

__all__ = [
    "AssetCandidateCommand",
    "AssetCandidateResult",
    "AssetVersionReadinessReference",
    "AssetVersionReference",
    "ProjectAssetSummary",
    "asset_version_exists",
    "create_or_link_candidate",
    "resolve_asset_version",
    "resolve_asset_version_readiness",
    "summarize_project_assets",
]

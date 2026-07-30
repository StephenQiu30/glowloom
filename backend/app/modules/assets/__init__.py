"""Public asset contracts and version-resolution use cases."""

from app.modules.assets.contracts import (
    AssetCandidateCommand,
    AssetCandidateResult,
    AssetVersionReference,
    ProjectAssetSummary,
)
from app.modules.assets.service import (
    asset_version_exists,
    create_or_link_candidate,
    resolve_asset_version,
    summarize_project_assets,
)

__all__ = [
    "AssetCandidateCommand",
    "AssetCandidateResult",
    "AssetVersionReference",
    "ProjectAssetSummary",
    "asset_version_exists",
    "create_or_link_candidate",
    "resolve_asset_version",
    "summarize_project_assets",
]

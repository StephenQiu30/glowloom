"""Public asset contracts and version-resolution use cases."""

from app.modules.assets.contracts import (
    AssetCandidateCommand,
    AssetCandidateResult,
    AssetVersionReference,
)
from app.modules.assets.service import (
    asset_version_exists,
    create_or_link_candidate,
    resolve_asset_version,
)

__all__ = [
    "AssetCandidateCommand",
    "AssetCandidateResult",
    "AssetVersionReference",
    "asset_version_exists",
    "create_or_link_candidate",
    "resolve_asset_version",
]

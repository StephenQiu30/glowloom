from __future__ import annotations

from pathlib import Path

import pytest

from app.modules.storygraph.bundle import BundleInvalid, StoryGraphBundle

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]


def test_bundle_hash_matches_the_frozen_manifest_and_only_allows_declared_files() -> None:
    bundle = StoryGraphBundle(REPOSITORY_ROOT)

    assert bundle.verify_installed_bundle() == bundle.manifest.skill_bundle_hash
    assert bundle.loaded_paths("extract_source_evidence") == (
        "SKILL.md",
        "references/source-evidence.md",
    )
    assert bundle.loaded_paths("analyze_story") == (
        "SKILL.md",
        "references/story-analysis.md",
        "references/entity-reconciliation.md",
    )


def test_bundle_fails_closed_for_missing_extra_non_utf8_and_symlink_files(tmp_path: Path) -> None:
    bundle_root = tmp_path / "agent/skills/build-storygraph"
    bundle_root.mkdir(parents=True)
    source = REPOSITORY_ROOT / "agent/skills/build-storygraph"
    for relative_path in StoryGraphBundle.known_paths():
        target = bundle_root / relative_path
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes((source / relative_path).read_bytes())

    bundle = StoryGraphBundle(tmp_path)
    assert bundle.compute_hash() == StoryGraphBundle(REPOSITORY_ROOT).compute_hash()

    (bundle_root / "extra.md").write_text("extra", encoding="utf-8")
    with pytest.raises(BundleInvalid):
        bundle.compute_hash()
    (bundle_root / "extra.md").unlink()

    (bundle_root / "references/source-evidence.md").write_bytes(b"\xff")
    with pytest.raises(BundleInvalid):
        bundle.compute_hash()

    (bundle_root / "references/source-evidence.md").unlink()
    (bundle_root / "references/source-evidence.md").symlink_to(
        REPOSITORY_ROOT / "agent/skills/build-storygraph/references/source-evidence.md"
    )
    with pytest.raises(BundleInvalid):
        bundle.compute_hash()


def test_review_stage_only_adds_the_explicit_reviewed_stage_references() -> None:
    bundle = StoryGraphBundle(REPOSITORY_ROOT)
    assert bundle.loaded_paths("review_storygraph", {"reviewed_stage": "draft_storyboard"}) == (
        "SKILL.md",
        "references/continuity-review.md",
        "references/storyboard-table.md",
        "references/visual-identity.md",
    )
    with pytest.raises(BundleInvalid):
        bundle.loaded_paths("review_storygraph", {"reviewed_stage": "production_bible"})

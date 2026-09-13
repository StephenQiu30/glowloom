from pathlib import Path

import pytest

from app.skills.catalog import SkillCatalog, SkillUnavailable

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]


def test_catalog_declares_the_current_agent_skill_capabilities() -> None:
    catalog = SkillCatalog(REPOSITORY_ROOT)
    assert [(item.key, item.bundle) for item in catalog.registrations()] == [
        ("storygraph", "build-storygraph"),
        ("scene_analysis", "build-storygraph"),
        ("visual_foundation", "build-storygraph"),
        ("vision_review", "build-storygraph"),
        ("text_storyboard", "text-storyboard"),
    ]


def test_catalog_verifies_every_installed_skill_release() -> None:
    catalog = SkillCatalog(REPOSITORY_ROOT)
    verified = catalog.verify_all()
    assert set(verified) == {
        "storygraph",
        "scene_analysis",
        "visual_foundation",
        "vision_review",
        "text_storyboard",
    }
    assert all(len(value) == 64 for value in verified.values())


def test_catalog_rejects_unknown_skill_without_path_fallback() -> None:
    catalog = SkillCatalog(REPOSITORY_ROOT)
    with pytest.raises(SkillUnavailable, match="skill_not_registered"):
        catalog.verify("../text-storyboard")


def test_catalog_never_uses_external_skill_directories() -> None:
    catalog = SkillCatalog(REPOSITORY_ROOT)
    bundles = {item.bundle for item in catalog.registrations()}
    assert bundles <= {"build-storygraph", "text-storyboard"}
    assert not (REPOSITORY_ROOT / ".agents" / "skills").exists()

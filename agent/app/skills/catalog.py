"""Explicit Skill registration and release verification for the Agent service."""

from __future__ import annotations

from collections.abc import Callable
from dataclasses import dataclass
from pathlib import Path
from typing import cast

from app.modules.storygraph.bundle import BundleInvalid, StoryGraphBundle
from app.modules.storygraph.scene_analysis_bundle import (
    SCENE_ANALYSIS_SKILL_BUNDLE_HASH,
    SceneAnalysisBundle,
)
from app.modules.text_storyboard.harness import RELEASE_HASH, TextSkill


class SkillUnavailable(ValueError):
    """Raised when a declared Skill release cannot be verified exactly."""


SkillFactory = Callable[[Path | None], object]
SkillVerifier = Callable[[object], str]


@dataclass(frozen=True)
class SkillRegistration:
    """One code-owned Skill capability and its immutable resource bundle."""

    key: str
    bundle: str
    expected_hash: str
    factory: SkillFactory
    verifier: SkillVerifier


def _storygraph(root: Path | None) -> StoryGraphBundle:
    return StoryGraphBundle(root)


def _scene_analysis(root: Path | None) -> SceneAnalysisBundle:
    return SceneAnalysisBundle(root)


def _text_storyboard(root: Path | None) -> TextSkill:
    from app.modules.text_storyboard import harness as text_harness

    if root is None:
        return text_harness.TextSkill()
    return text_harness.TextSkill(root / "agent" / "skills" / "text-storyboard")


def _verify_storygraph(value: object) -> str:
    return cast(StoryGraphBundle, value).verify_installed_bundle()


def _verify_scene_analysis(value: object) -> str:
    return cast(SceneAnalysisBundle, value).verify_installed_bundle()


def _verify_text_storyboard(value: object) -> str:
    return cast(TextSkill, value).release_hash()


class SkillCatalog:
    """The closed set of Skill capabilities shipped by this Agent image."""

    def __init__(self, repository_root: Path | None = None) -> None:
        self.repository_root = repository_root or Path(__file__).resolve().parents[3]
        self._registrations = (
            SkillRegistration(
                key="storygraph",
                bundle="build-storygraph",
                expected_hash=StoryGraphBundle().manifest.skill_bundle_hash,
                factory=_storygraph,
                verifier=_verify_storygraph,
            ),
            SkillRegistration(
                key="scene_analysis",
                bundle="build-storygraph",
                expected_hash=SCENE_ANALYSIS_SKILL_BUNDLE_HASH,
                factory=_scene_analysis,
                verifier=_verify_scene_analysis,
            ),
            SkillRegistration(
                key="text_storyboard",
                bundle="text-storyboard",
                expected_hash=RELEASE_HASH,
                factory=_text_storyboard,
                verifier=_verify_text_storyboard,
            ),
        )

    def registrations(self) -> tuple[SkillRegistration, ...]:
        return self._registrations

    def registration(self, key: str) -> SkillRegistration:
        for registration in self._registrations:
            if registration.key == key:
                return registration
        raise SkillUnavailable("skill_not_registered")

    def load(self, key: str) -> object:
        """Construct a registered Skill runtime without accepting arbitrary paths."""

        return self.registration(key).factory(self.repository_root)

    def verify(self, key: str) -> str:
        registration = self.registration(key)
        try:
            actual = registration.verifier(registration.factory(self.repository_root))
        except (BundleInvalid, OSError, UnicodeDecodeError, ValueError) as error:
            raise SkillUnavailable("skill_release_unavailable") from error
        if actual != registration.expected_hash:
            raise SkillUnavailable("skill_release_unavailable")
        return actual

    def verify_all(self) -> dict[str, str]:
        verified: dict[str, str] = {}
        for registration in self._registrations:
            verified[registration.key] = self.verify(registration.key)
        return verified

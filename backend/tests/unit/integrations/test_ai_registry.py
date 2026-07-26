from __future__ import annotations

from dataclasses import FrozenInstanceError
from typing import cast

import pytest

from integrations.ai.deterministic_media import (
    DeterministicImageProvider,
    DeterministicTtsProvider,
)
from integrations.ai.deterministic_text import DeterministicTextProvider
from integrations.ai.deterministic_video import DeterministicVideoProvider
from integrations.ai.registry import (
    AdapterUnavailable,
    AiModelProfile,
    AiModelRegistry,
    DefaultProfileMissing,
    ProfileCapabilityMismatch,
    ProfileConfigurationError,
    ProfileDisabled,
    ProfileNotFound,
    ProfileSchemaUnsupported,
    create_mvp_registry,
)


def profile(
    profile_id: str,
    *,
    capability: str = "text",
    enabled: bool = True,
) -> AiModelProfile:
    return AiModelProfile(
        profile_id=profile_id,
        capability=capability,
        provider_id="mock",
        model_id=f"model-{profile_id}",
        route_version="route-v1",
        schema_versions=frozenset({"script-v1"}),
        parameters={"temperature": 0, "nested": {"stop": ["END"]}},
        kind="mock",
        enabled=enabled,
    )


def test_mvp_registry_has_exact_four_capabilities_and_safe_defaults() -> None:
    registry = create_mvp_registry()

    assert registry.capabilities == frozenset({"text", "image", "video", "tts"})
    assert {
        capability: registry.resolve(capability).profile_id for capability in registry.capabilities
    } == {
        "text": "mock-text-v1",
        "image": "mock-image-v1",
        "video": "mock-video-v1",
        "tts": "mock-tts-v1",
    }
    assert {item["profile_id"] for item in registry.public_profiles()} == {
        "mock-text-v1",
        "mock-image-v1",
        "mock-video-v1",
        "mock-tts-v1",
    }
    assert all(
        "credential" not in item and "secret" not in item for item in registry.public_profiles()
    )


def test_profile_is_deeply_immutable_and_rejects_secret_material() -> None:
    item = profile("text-a")

    with pytest.raises(FrozenInstanceError):
        item.model_id = "changed"  # type: ignore[misc]
    with pytest.raises(TypeError):
        item.parameters["temperature"] = 1  # type: ignore[index]
    nested = cast(dict[str, object], item.parameters["nested"])
    with pytest.raises(TypeError):
        nested["stop"] = ()

    with pytest.raises(ProfileConfigurationError, match="secret material"):
        AiModelProfile(
            profile_id="unsafe",
            capability="text",
            provider_id="provider",
            model_id="model",
            route_version="route-v1",
            schema_versions=frozenset({"script-v1"}),
            parameters={"api_key": "must-not-be-here"},
            kind="provider",
            credential_env_names=("PROVIDER_API_KEY",),
        )


def test_exact_resolution_never_falls_back_to_another_profile() -> None:
    registry = AiModelRegistry(
        [
            profile("text-a"),
            profile("text-disabled", enabled=False),
            profile("image-a", capability="image"),
        ],
        defaults={"text": "text-a"},
    )

    assert registry.resolve("text", "text-a").profile_id == "text-a"
    with pytest.raises(ProfileDisabled):
        registry.resolve("text", "text-disabled")
    with pytest.raises(ProfileNotFound):
        registry.resolve("text", "missing")
    with pytest.raises(ProfileCapabilityMismatch):
        registry.resolve("text", "image-a")
    with pytest.raises(DefaultProfileMissing):
        registry.resolve("video")


def test_selection_freezes_profile_fields_and_enforces_schema() -> None:
    registry = AiModelRegistry([profile("text-a")], defaults={"text": "text-a"})

    selection = registry.select("text", schema_version="script-v1")
    assert selection.model_profile_id == "text-a"
    assert selection.provider_id == "mock"
    assert selection.model_id == "model-text-a"
    assert selection.route_version == "route-v1"
    assert selection.schema_version == "script-v1"
    assert selection.parameters == {
        "temperature": 0,
        "nested": {"stop": ["END"]},
    }
    with pytest.raises(ProfileSchemaUnsupported):
        registry.select("text", schema_version="storyboard-v2")


def test_adapter_factory_is_exact_and_missing_factory_is_an_error() -> None:
    calls: list[str] = []

    def factory(item: AiModelProfile) -> object:
        calls.append(item.profile_id)
        return {"adapter_for": item.profile_id}

    registry = AiModelRegistry(
        [profile("text-a"), profile("image-a", capability="image")],
        defaults={"text": "text-a", "image": "image-a"},
        adapter_factories={("text", "mock"): factory},
    )

    binding = registry.bind("text", "text-a")
    assert binding.adapter == {"adapter_for": "text-a"}
    assert calls == ["text-a"]
    with pytest.raises(AdapterUnavailable):
        registry.bind("image", "image-a")


def test_mvp_registry_binds_all_four_deterministic_adapters() -> None:
    registry = create_mvp_registry()

    assert isinstance(registry.bind("text").adapter, DeterministicTextProvider)
    assert isinstance(registry.bind("image").adapter, DeterministicImageProvider)
    assert isinstance(registry.bind("video").adapter, DeterministicVideoProvider)
    assert isinstance(registry.bind("tts").adapter, DeterministicTtsProvider)
    assert registry.bind("image").adapter is not registry.bind("image").adapter


def test_duplicate_profile_ids_are_rejected() -> None:
    with pytest.raises(ProfileConfigurationError, match="duplicate"):
        AiModelRegistry([profile("text-a"), profile("text-a")])

from __future__ import annotations

from collections.abc import Callable, Iterable, Mapping
from dataclasses import dataclass
from types import MappingProxyType

from integrations.ai.profiles import (
    AiModelProfile,
    AiModelSelection,
    Capability,
    DefaultProfileMissing,
    ProfileCapabilityMismatch,
    ProfileConfigurationError,
    ProfileDisabled,
    ProfileNotFound,
    ProfileSchemaUnsupported,
    thaw_json,
)

AdapterFactory = Callable[[AiModelProfile], object]


class AdapterUnavailable(LookupError):
    pass


@dataclass(frozen=True, slots=True)
class AiModelBinding:
    profile: AiModelProfile
    adapter: object


class AiModelRegistry:
    def __init__(
        self,
        profiles: Iterable[AiModelProfile],
        *,
        defaults: Mapping[Capability, str] | None = None,
        adapter_factories: Mapping[tuple[Capability, str], AdapterFactory] | None = None,
    ) -> None:
        indexed: dict[str, AiModelProfile] = {}
        for profile in profiles:
            if profile.profile_id in indexed:
                raise ProfileConfigurationError("duplicate AI profile id")
            indexed[profile.profile_id] = profile
        self._profiles = MappingProxyType(indexed)
        self._defaults = MappingProxyType(dict(defaults or {}))
        self._factories = MappingProxyType(dict(adapter_factories or {}))
        for capability, profile_id in self._defaults.items():
            default_profile = self._profiles.get(profile_id)
            if default_profile is None or default_profile.capability != capability:
                raise ProfileConfigurationError("default AI profile is invalid")

    @property
    def capabilities(self) -> frozenset[Capability]:
        return frozenset(profile.capability for profile in self._profiles.values())

    def resolve(self, capability: Capability, profile_id: str | None = None) -> AiModelProfile:
        selected_id = profile_id or self._defaults.get(capability)
        if selected_id is None:
            raise DefaultProfileMissing(f"no default profile for {capability}")
        profile = self._profiles.get(selected_id)
        if profile is None:
            raise ProfileNotFound(selected_id)
        if profile.capability != capability:
            raise ProfileCapabilityMismatch(selected_id)
        if not profile.enabled:
            raise ProfileDisabled(selected_id)
        return profile

    def select(
        self,
        capability: Capability,
        profile_id: str | None = None,
        *,
        schema_version: str,
    ) -> AiModelSelection:
        profile = self.resolve(capability, profile_id)
        if schema_version not in profile.schema_versions:
            raise ProfileSchemaUnsupported(schema_version)
        parameters = thaw_json(profile.parameters)
        assert isinstance(parameters, dict)
        return AiModelSelection(
            model_profile_id=profile.profile_id,
            provider_id=profile.provider_id,
            model_id=profile.model_id,
            route_version=profile.route_version,
            schema_version=schema_version,
            parameters=parameters,
        )

    def bind(self, capability: Capability, profile_id: str | None = None) -> AiModelBinding:
        profile = self.resolve(capability, profile_id)
        factory = self._factories.get((capability, profile.provider_id))
        if factory is None:
            raise AdapterUnavailable(profile.profile_id)
        return AiModelBinding(profile=profile, adapter=factory(profile))

    def public_profiles(self) -> tuple[dict[str, object], ...]:
        return tuple(
            self._profiles[profile_id].public_metadata() for profile_id in sorted(self._profiles)
        )


def create_mvp_registry(
    adapter_factories: Mapping[tuple[Capability, str], AdapterFactory] | None = None,
) -> AiModelRegistry:
    profiles = (
        AiModelProfile(
            "mock-text-v1",
            "text",
            "mock",
            "deterministic-text",
            "text-route-v1",
            frozenset({"script-v1", "storyboard-generation-v1"}),
            {"temperature": 0, "response_format": "json"},
            "mock",
        ),
        AiModelProfile(
            "mock-image-v1",
            "image",
            "mock",
            "deterministic-image",
            "image-route-v1",
            frozenset({"image-v1"}),
            {"width": 720, "height": 1280, "format": "png"},
            "mock",
        ),
        AiModelProfile(
            "mock-video-v1",
            "video",
            "mock",
            "deterministic-video",
            "video-route-v1",
            frozenset({"video-v1"}),
            {"width": 720, "height": 1280, "fps": 24, "codec": "h264"},
            "mock",
        ),
        AiModelProfile(
            "mock-tts-v1",
            "tts",
            "mock",
            "deterministic-tts",
            "tts-route-v1",
            frozenset({"tts-v1"}),
            {"sample_rate": 48000, "channels": 1, "format": "wav"},
            "mock",
        ),
    )
    defaults = {profile.capability: profile.profile_id for profile in profiles}
    return AiModelRegistry(
        profiles,
        defaults=defaults,
        adapter_factories=adapter_factories,
    )

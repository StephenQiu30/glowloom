from __future__ import annotations

import re
from collections.abc import Mapping
from dataclasses import dataclass
from types import MappingProxyType
from typing import Literal

Capability = Literal["text", "image", "video", "tts"]
ProfileKind = Literal["mock", "provider"]

CAPABILITIES = frozenset({"text", "image", "video", "tts"})
IDENTIFIER = re.compile(r"^[a-z][a-z0-9._-]{1,127}$")
ENV_NAME = re.compile(r"^[A-Z][A-Z0-9_]*$")
SECRET_KEYS = frozenset(
    {"api_key", "access_token", "auth_token", "secret", "password", "credentials"}
)


class ProfileConfigurationError(ValueError):
    pass


class ProfileNotFound(LookupError):
    pass


class DefaultProfileMissing(LookupError):
    pass


class ProfileCapabilityMismatch(ValueError):
    pass


class ProfileDisabled(ValueError):
    pass


class ProfileSchemaUnsupported(ValueError):
    pass


def _freeze_json(value: object) -> object:
    if value is None or isinstance(value, (str, int, float, bool)):
        return value
    if isinstance(value, Mapping):
        frozen: dict[str, object] = {}
        for key, item in value.items():
            if not isinstance(key, str):
                raise ProfileConfigurationError("profile parameter keys must be strings")
            if key.lower() in SECRET_KEYS:
                raise ProfileConfigurationError("profile parameters contain secret material")
            frozen[key] = _freeze_json(item)
        return MappingProxyType(frozen)
    if isinstance(value, (list, tuple)):
        return tuple(_freeze_json(item) for item in value)
    raise ProfileConfigurationError("profile parameters must contain JSON values")


def thaw_json(value: object) -> object:
    if isinstance(value, Mapping):
        return {str(key): thaw_json(item) for key, item in value.items()}
    if isinstance(value, tuple):
        return [thaw_json(item) for item in value]
    return value


@dataclass(frozen=True, slots=True)
class AiModelProfile:
    profile_id: str
    capability: Capability
    provider_id: str
    model_id: str
    route_version: str
    schema_versions: frozenset[str]
    parameters: Mapping[str, object]
    kind: ProfileKind
    enabled: bool = True
    credential_env_names: tuple[str, ...] = ()

    def __post_init__(self) -> None:
        identifiers = (
            self.profile_id,
            self.provider_id,
            self.model_id,
            self.route_version,
        )
        if self.capability not in CAPABILITIES:
            raise ProfileConfigurationError("unsupported AI capability")
        if any(IDENTIFIER.fullmatch(value) is None for value in identifiers):
            raise ProfileConfigurationError("profile identifiers are invalid")
        schemas = frozenset(self.schema_versions)
        if not schemas or any(IDENTIFIER.fullmatch(value) is None for value in schemas):
            raise ProfileConfigurationError("profile schema versions are invalid")
        credentials = tuple(self.credential_env_names)
        if len(credentials) != len(set(credentials)) or any(
            ENV_NAME.fullmatch(value) is None for value in credentials
        ):
            raise ProfileConfigurationError("credential environment names are invalid")
        frozen = _freeze_json(self.parameters)
        if not isinstance(frozen, Mapping):
            raise ProfileConfigurationError("profile parameters must be an object")
        object.__setattr__(self, "schema_versions", schemas)
        object.__setattr__(self, "parameters", frozen)
        object.__setattr__(self, "credential_env_names", credentials)

    def public_metadata(self) -> dict[str, object]:
        return {
            "profile_id": self.profile_id,
            "capability": self.capability,
            "provider_id": self.provider_id,
            "model_id": self.model_id,
            "route_version": self.route_version,
            "schema_versions": sorted(self.schema_versions),
            "parameters": thaw_json(self.parameters),
            "kind": self.kind,
            "enabled": self.enabled,
        }


@dataclass(frozen=True, slots=True)
class AiModelSelection:
    model_profile_id: str
    provider_id: str
    model_id: str
    route_version: str
    schema_version: str
    parameters: dict[str, object]

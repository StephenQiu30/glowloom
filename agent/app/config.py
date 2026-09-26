"""Process configuration, read from LV_* environment variables and validated at startup."""

from typing import Literal

from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_prefix="LV_", extra="ignore")

    env: Literal["local", "staging", "prod"] = "local"
    log_level: Literal["debug", "info", "warn", "error"] = "info"

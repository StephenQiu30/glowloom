from __future__ import annotations

from functools import lru_cache
from typing import Literal

from pydantic import AliasChoices, Field
from pydantic_settings import BaseSettings, SettingsConfigDict


class ApplicationSettings(BaseSettings):
    model_config = SettingsConfigDict(
        env_prefix="LANVERSE_",
        case_sensitive=False,
        extra="ignore",
    )

    environment: Literal["development", "test", "production"] = "development"
    api_host: str = "127.0.0.1"
    api_port: int = Field(default=8000, ge=1, le=65535)
    log_level: Literal["DEBUG", "INFO", "WARNING", "ERROR", "CRITICAL"] = "INFO"
    docs_enabled: bool | None = None
    database_url: str | None = Field(
        default=None,
        validation_alias=AliasChoices("DATABASE_URL", "LANVERSE_DATABASE_URL"),
    )
    database_pool_min_size: int = Field(default=1, ge=1, le=20)
    database_pool_max_size: int = Field(default=10, ge=1, le=50)
    provider_max_concurrency: int = Field(default=3, ge=1, le=3)
    provider_submit_timeout_seconds: int = Field(default=30, ge=1, le=30)
    provider_status_timeout_seconds: int = Field(default=10, ge=1, le=10)
    provider_poll_min_seconds: int = Field(default=2, ge=2, le=2)
    provider_poll_max_seconds: int = Field(default=10, ge=2, le=10)
    text_task_timeout_seconds: int = Field(default=120, ge=1, le=120)
    video_task_timeout_seconds: int = Field(default=600, ge=1, le=600)
    text_status_poll_limit: int = Field(default=60, ge=1, le=60)
    video_status_poll_limit: int = Field(default=300, ge=1, le=300)
    worker_lease_seconds: int = Field(default=30, ge=3, le=120)
    worker_heartbeat_seconds: int = Field(default=10, ge=1, le=30)

    @property
    def expose_docs(self) -> bool:
        if self.docs_enabled is not None:
            return self.docs_enabled
        return self.environment != "production"

    def require_database_url(self) -> str:
        if not self.database_url:
            raise ValueError("DATABASE_URL is required")
        if not self.database_url.startswith(("postgresql://", "postgres://")):
            raise ValueError("DATABASE_URL must use PostgreSQL")
        return self.database_url


@lru_cache(maxsize=1)
def load_settings() -> ApplicationSettings:
    return ApplicationSettings()

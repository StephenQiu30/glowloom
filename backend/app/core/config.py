from functools import lru_cache
from pathlib import Path
from typing import Literal

from pydantic import Field, SecretStr, field_validator, model_validator
from pydantic_settings import BaseSettings, SettingsConfigDict

REPOSITORY_ENV_FILE = Path(__file__).resolve().parents[3] / ".env"


class Settings(BaseSettings):
    model_config = SettingsConfigDict(
        env_file=REPOSITORY_ENV_FILE,
        env_file_encoding="utf-8",
        extra="ignore",
        case_sensitive=False,
    )

    app_name: str = "Lanverse API"
    environment: Literal["development", "test", "production"] = "development"
    log_level: str = "INFO"
    api_host: str = "0.0.0.0"
    api_port: int = Field(default=8000, ge=1, le=65535)
    cors_origins: list[str] = ["http://localhost:3000", "http://127.0.0.1:3000"]

    database_url: str = "postgresql+asyncpg://postgres@127.0.0.1:5432/lanverse"
    test_database_url: str | None = None
    redis_url: str = "redis://127.0.0.1:6379/0"
    workspace_cache_ttl_seconds: int = Field(default=60, ge=1, le=3600)
    cache_ttl_jitter_seconds: int = Field(default=10, ge=0, le=300)
    generation_high_cost_window_seconds: int = Field(default=60, ge=1, le=3600)
    generation_high_cost_workspace_limit: int = Field(default=3, ge=1, le=1000)
    generation_high_cost_global_limit: int = Field(default=20, ge=1, le=10000)
    generation_high_cost_idempotency_ttl_seconds: int = Field(
        default=900,
        ge=1,
        le=86400,
    )
    rabbitmq_url: str = "amqp://guest:guest@127.0.0.1:5672/"
    outbox_batch_size: int = Field(default=20, ge=1, le=100)
    outbox_claim_seconds: int = Field(default=60, ge=5, le=3600)
    outbox_poll_seconds: float = Field(default=1.0, ge=0.1, le=60)

    minio_endpoint: str = "127.0.0.1:9000"
    minio_access_key: str = "lanverse"
    minio_secret_key: str = "lanverse-development-only"
    minio_bucket: str = "lanverse-media"
    minio_secure: bool = False
    infrastructure_timeout_seconds: float = Field(default=1.5, gt=0, le=10)
    storage_thread_limit: int = Field(default=4, ge=1, le=32)
    storage_operation_timeout_seconds: float = Field(default=3, ge=0.1, le=30)
    media_max_upload_bytes: int = Field(default=2 * 1024 * 1024 * 1024, ge=1)
    media_upload_ttl_seconds: int = Field(default=900, ge=60, le=3600)
    media_access_ttl_seconds: int = Field(default=300, ge=30, le=900)
    media_probe_timeout_seconds: int = Field(default=120, ge=5, le=600)
    media_cleanup_interval_seconds: int = Field(default=3600, ge=60, le=86400)
    media_cleanup_batch_size: int = Field(default=100, ge=1, le=500)
    media_location_rollback_seconds: int = Field(
        default=86400, ge=60, le=7 * 24 * 60 * 60
    )

    jwt_secret_key: SecretStr = SecretStr(
        "development-only-jwt-secret-change-before-production"
    )
    jwt_issuer: str = "lanverse-api"
    jwt_audience: str = "lanverse-web"
    jwt_access_token_minutes: int = Field(default=30, ge=5, le=1440)
    deepseek_api_key: SecretStr | None = None
    ark_api_key: SecretStr | None = None

    @field_validator("deepseek_api_key", "ark_api_key", mode="before")
    @classmethod
    def empty_provider_key_is_unconfigured(cls, value: object) -> object:
        if isinstance(value, str) and not value.strip():
            return None
        return value

    @model_validator(mode="after")
    def reject_development_jwt_secret_in_production(self) -> "Settings":
        if self.cache_ttl_jitter_seconds >= self.workspace_cache_ttl_seconds:
            raise ValueError(
                "CACHE_TTL_JITTER_SECONDS must be lower than WORKSPACE_CACHE_TTL_SECONDS"
            )
        if self.generation_high_cost_global_limit < self.generation_high_cost_workspace_limit:
            raise ValueError(
                "GENERATION_HIGH_COST_GLOBAL_LIMIT must not be lower than "
                "GENERATION_HIGH_COST_WORKSPACE_LIMIT"
            )
        if (
            self.generation_high_cost_idempotency_ttl_seconds
            < self.generation_high_cost_window_seconds
        ):
            raise ValueError(
                "GENERATION_HIGH_COST_IDEMPOTENCY_TTL_SECONDS must not be lower than "
                "GENERATION_HIGH_COST_WINDOW_SECONDS"
            )
        if (
            self.environment == "production"
            and self.jwt_secret_key.get_secret_value()
            == "development-only-jwt-secret-change-before-production"
        ):
            raise ValueError("JWT_SECRET_KEY must be set in production")
        return self


@lru_cache
def get_settings() -> Settings:
    return Settings()

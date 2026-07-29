from functools import lru_cache
from pathlib import Path
from typing import Literal

from pydantic import Field, SecretStr, model_validator
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
    cors_origins: list[str] = ["http://localhost:3000", "http://127.0.0.1:3000"]

    database_url: str = "postgresql+asyncpg://postgres@127.0.0.1:5432/lanverse"
    test_database_url: str | None = None
    redis_url: str = "redis://127.0.0.1:6379/0"
    rabbitmq_url: str = "amqp://lanverse:lanverse-development-only@127.0.0.1:5672/"

    minio_endpoint: str = "127.0.0.1:9000"
    minio_access_key: str = "lanverse"
    minio_secret_key: str = "lanverse-development-only"
    minio_bucket: str = "lanverse-media"
    minio_secure: bool = False
    infrastructure_timeout_seconds: float = Field(default=1.5, gt=0, le=10)
    storage_thread_limit: int = Field(default=4, ge=1, le=32)

    jwt_secret_key: SecretStr = SecretStr(
        "development-only-jwt-secret-change-before-production"
    )
    jwt_issuer: str = "lanverse-api"
    jwt_audience: str = "lanverse-web"
    jwt_access_token_minutes: int = Field(default=30, ge=5, le=1440)

    @model_validator(mode="after")
    def reject_development_jwt_secret_in_production(self) -> "Settings":
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

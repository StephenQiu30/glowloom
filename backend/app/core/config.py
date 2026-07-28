from functools import lru_cache
from typing import Literal

from pydantic import Field
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    model_config = SettingsConfigDict(
        env_file=".env",
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


@lru_cache
def get_settings() -> Settings:
    return Settings()

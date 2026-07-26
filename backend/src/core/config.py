from __future__ import annotations

from dataclasses import dataclass, field
from functools import lru_cache
from typing import Literal
from urllib.parse import urlsplit

from pydantic import AliasChoices, Field
from pydantic_settings import BaseSettings, SettingsConfigDict


@dataclass(frozen=True, slots=True)
class MinioConfig:
    endpoint: str
    public_endpoint: str
    bucket: str
    access_key: str = field(repr=False)
    secret_key: str = field(repr=False)
    secure: bool = False


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
    minio_endpoint: str | None = Field(
        default=None,
        validation_alias=AliasChoices("MINIO_ENDPOINT", "LANVERSE_MINIO_ENDPOINT"),
    )
    minio_public_endpoint: str | None = Field(
        default=None,
        validation_alias=AliasChoices(
            "MINIO_PUBLIC_ENDPOINT", "LANVERSE_MINIO_PUBLIC_ENDPOINT"
        ),
    )
    minio_bucket: str | None = Field(
        default=None,
        validation_alias=AliasChoices("MINIO_BUCKET", "LANVERSE_MINIO_BUCKET"),
    )
    minio_access_key: str | None = Field(
        default=None,
        validation_alias=AliasChoices("MINIO_ACCESS_KEY", "LANVERSE_MINIO_ACCESS_KEY"),
    )
    minio_secret_key: str | None = Field(
        default=None,
        validation_alias=AliasChoices("MINIO_SECRET_KEY", "LANVERSE_MINIO_SECRET_KEY"),
    )
    minio_secure: bool = Field(
        default=False,
        validation_alias=AliasChoices("MINIO_SECURE", "LANVERSE_MINIO_SECURE"),
    )
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
    render_runtime_image: str | None = Field(
        default=None,
        pattern=r"^(?:[a-z0-9./_-]+@sha256:[0-9a-f]{64}|sha256:[0-9a-f]{64})$",
    )

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

    @property
    def has_minio_configuration(self) -> bool:
        return any(
            value is not None
            for value in (
                self.minio_endpoint,
                self.minio_public_endpoint,
                self.minio_bucket,
                self.minio_access_key,
                self.minio_secret_key,
            )
        )

    def require_minio_config(self) -> MinioConfig:
        values = {
            "MINIO_ENDPOINT": self.minio_endpoint,
            "MINIO_PUBLIC_ENDPOINT": self.minio_public_endpoint,
            "MINIO_BUCKET": self.minio_bucket,
            "MINIO_ACCESS_KEY": self.minio_access_key,
            "MINIO_SECRET_KEY": self.minio_secret_key,
        }
        missing = [name for name, value in values.items() if not value]
        if missing:
            raise ValueError(f"required MinIO variables are missing: {', '.join(missing)}")
        endpoint = self.minio_endpoint or ""
        public_endpoint = self.minio_public_endpoint or ""
        bucket = self.minio_bucket or ""
        parsed_public = urlsplit(public_endpoint)
        if "://" in endpoint or "/" in endpoint or endpoint != endpoint.strip():
            raise ValueError("MINIO_ENDPOINT must be a trimmed host[:port]")
        if (
            parsed_public.scheme not in {"http", "https"}
            or not parsed_public.netloc
            or parsed_public.username is not None
            or parsed_public.password is not None
            or parsed_public.path not in {"", "/"}
        ):
            raise ValueError("MINIO_PUBLIC_ENDPOINT must be an HTTP(S) origin without credentials")
        if bucket != bucket.strip():
            raise ValueError("MINIO_BUCKET must be trimmed")
        return MinioConfig(
            endpoint=endpoint,
            public_endpoint=public_endpoint.rstrip("/"),
            bucket=bucket,
            access_key=self.minio_access_key or "",
            secret_key=self.minio_secret_key or "",
            secure=self.minio_secure,
        )


@lru_cache(maxsize=1)
def load_settings() -> ApplicationSettings:
    return ApplicationSettings()

from functools import lru_cache
from pathlib import Path
from typing import Literal

from pydantic import Field, SecretStr, field_validator, model_validator
from pydantic_settings import BaseSettings, SettingsConfigDict

REPOSITORY_ENV_FILE = Path(__file__).resolve().parents[3] / ".env"
DEVELOPMENT_CORS_ORIGINS = (
    "http://localhost:8123",
    "http://127.0.0.1:8123",
)
DEVELOPMENT_EMAIL_VERIFICATION_HMAC_SECRET = (
    "development-only-registration-hmac-change-before-production"
)


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
    api_port: int = Field(default=8686, ge=1, le=65535)
    cors_origins: list[str] = list(DEVELOPMENT_CORS_ORIGINS)

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
    smtp_enabled: bool = False
    smtp_host: str | None = None
    smtp_port: int = Field(default=465, ge=1, le=65535)
    smtp_tls_mode: Literal["tls", "starttls"] = "tls"
    smtp_username: str | None = None
    smtp_password: SecretStr | None = None
    smtp_from_email: str | None = None
    smtp_from_name: str = "Lanverse"
    smtp_timeout_seconds: float = Field(default=10, gt=0, le=30)
    email_verification_hmac_secret: SecretStr = SecretStr(
        DEVELOPMENT_EMAIL_VERIFICATION_HMAC_SECRET
    )
    email_verification_ttl_seconds: int = Field(default=600, ge=60, le=1800)
    email_verification_resend_seconds: int = Field(default=60, ge=10, le=600)
    email_verification_max_attempts: int = Field(default=5, ge=1, le=10)
    email_verification_ticket_ttl_seconds: int = Field(
        default=600, ge=60, le=1800
    )
    email_verification_source_window_seconds: int = Field(
        default=3600, ge=60, le=86400
    )
    email_verification_source_limit: int = Field(default=20, ge=1, le=1000)
    deepseek_api_key: SecretStr | None = None
    ark_api_key: SecretStr | None = None
    provider_credential_key_id: str = Field(
        default="local-provider-v1", min_length=1, max_length=100
    )
    provider_credential_master_key: SecretStr | None = None
    provider_credential_fingerprint_key: SecretStr | None = None

    @field_validator(
        "deepseek_api_key",
        "ark_api_key",
        "provider_credential_master_key",
        "provider_credential_fingerprint_key",
        "smtp_host",
        "smtp_username",
        "smtp_password",
        "smtp_from_email",
        mode="before",
    )
    @classmethod
    def empty_provider_key_is_unconfigured(cls, value: object) -> object:
        if isinstance(value, str) and not value.strip():
            return None
        return value

    @model_validator(mode="after")
    def validate_cross_setting_invariants(self) -> "Settings":
        if self.environment == "development":
            self.cors_origins = list(
                dict.fromkeys((*self.cors_origins, *DEVELOPMENT_CORS_ORIGINS))
            )
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
        verification_secret = self.email_verification_hmac_secret.get_secret_value()
        if len(verification_secret.encode("utf-8")) < 32:
            raise ValueError(
                "EMAIL_VERIFICATION_HMAC_SECRET must contain at least 32 bytes"
            )
        if verification_secret == self.jwt_secret_key.get_secret_value():
            raise ValueError(
                "EMAIL_VERIFICATION_HMAC_SECRET must not reuse JWT_SECRET_KEY"
            )
        if (
            self.environment == "production"
            and verification_secret == DEVELOPMENT_EMAIL_VERIFICATION_HMAC_SECRET
        ):
            raise ValueError(
                "EMAIL_VERIFICATION_HMAC_SECRET must be set in production"
            )
        if self.smtp_enabled:
            required: dict[str, object | None] = {
                "SMTP_HOST": self.smtp_host,
                "SMTP_FROM_EMAIL": self.smtp_from_email,
            }
            if self.environment == "production":
                required.update(
                    {
                        "SMTP_USERNAME": self.smtp_username,
                        "SMTP_PASSWORD": self.smtp_password,
                    }
                )
            missing = [name for name, value in required.items() if value is None]
            if missing:
                raise ValueError(
                    f"{', '.join(missing)} must be set when SMTP_ENABLED is true"
                )
        if (self.smtp_username is None) != (self.smtp_password is None):
            raise ValueError(
                "SMTP_USERNAME and SMTP_PASSWORD must be configured together"
            )
        if (
            self.provider_credential_master_key is not None
            and self.provider_credential_fingerprint_key is not None
            and self.provider_credential_master_key.get_secret_value()
            == self.provider_credential_fingerprint_key.get_secret_value()
        ):
            raise ValueError(
                "PROVIDER_CREDENTIAL_FINGERPRINT_KEY must not reuse "
                "PROVIDER_CREDENTIAL_MASTER_KEY"
            )
        return self


@lru_cache
def get_settings() -> Settings:
    return Settings()

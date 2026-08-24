import asyncio
from collections.abc import Awaitable, Callable
from contextlib import asynccontextmanager

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import Response

from app.core.config import Settings, get_settings
from app.core.database import database_ping
from app.core.errors import register_exception_handlers
from app.core.logging import RequestContextMiddleware, configure_logging
from app.core.observability import MetricsMiddleware, metrics_response
from app.core.schemas import DependencyStatus, HealthResponse, ReadinessResponse
from app.core.telemetry import configure_telemetry
from app.integrations.kafka import kafka_ping
from app.integrations.minio import MinioObjectStorage
from app.integrations.redis import RedisCache, redis_ping
from app.integrations.redis_auth_sessions import RedisAuthSessionStore
from app.modules.assets.api import router as assets_router
from app.modules.governance.api import router as governance_router
from app.modules.identity.api import router as identity_router
from app.modules.identity.registration_verifications.crypto import (
    generate_registration_code,
)
from app.modules.identity.registration_verifications.redis_store import (
    RedisRegistrationVerificationStore,
)
from app.modules.identity.registration_verifications.smtp import (
    SMTPRegistrationMailer,
)
from app.modules.media.api import router as media_router
from app.modules.production.api import router as production_router
from app.modules.projects.api import router as projects_router
from app.modules.scheduling.api import router as scheduling_router
from app.modules.scripts.api import router as scripts_router
from app.modules.storyboards.api import router as storyboards_router

Check = Callable[[], Awaitable[None]]


@asynccontextmanager
async def lifespan(app: FastAPI):
    configure_logging(
        app.state.settings.log_level,
        environment=app.state.settings.environment,
    )
    try:
        yield
    finally:
        await app.state.registration_verification_store.close()
        await app.state.auth_session_store.close()
        await app.state.cache_port.close()


def create_app(settings: Settings | None = None) -> FastAPI:
    active = settings or get_settings()
    configure_telemetry(
        service_name="lanverse-api",
        environment=active.environment,
    )
    app = FastAPI(title=active.app_name, version="0.1.0", lifespan=lifespan)
    app.state.settings = active
    redis = RedisCache(
        active.redis_url,
        environment=active.environment,
        socket_timeout_seconds=min(active.infrastructure_timeout_seconds, 0.25),
    )
    app.state.cache_port = redis
    app.state.high_cost_guard = redis
    app.state.auth_session_store = RedisAuthSessionStore(
        active.redis_url,
        environment=active.environment,
        socket_timeout_seconds=min(active.infrastructure_timeout_seconds, 0.25),
    )
    app.state.registration_verification_store = RedisRegistrationVerificationStore(
        active.redis_url,
        environment=active.environment,
        socket_timeout_seconds=min(active.infrastructure_timeout_seconds, 0.25),
    )
    app.state.registration_mailer = SMTPRegistrationMailer(
        enabled=active.smtp_enabled,
        host=active.smtp_host,
        port=active.smtp_port,
        tls_mode=active.smtp_tls_mode,
        username=active.smtp_username,
        password=active.smtp_password,
        from_email=active.smtp_from_email,
        from_name=active.smtp_from_name,
        timeout_seconds=active.smtp_timeout_seconds,
    )
    app.state.registration_code_generator = generate_registration_code
    app.add_middleware(MetricsMiddleware)
    app.add_middleware(RequestContextMiddleware)
    app.add_middleware(
        CORSMiddleware,
        allow_origins=active.cors_origins,
        allow_credentials=True,
        allow_methods=["*"],
        allow_headers=["*"],
    )
    register_exception_handlers(app)
    app.include_router(assets_router)
    app.include_router(identity_router)
    app.include_router(governance_router)
    app.include_router(media_router)
    app.include_router(projects_router)
    app.include_router(production_router)
    app.include_router(scripts_router)
    app.include_router(scheduling_router)
    app.include_router(storyboards_router)

    @app.get("/healthz", response_model=HealthResponse, tags=["system"])
    # FastAPI registers route functions through decorators at runtime.
    async def _healthz() -> HealthResponse:  # pyright: ignore[reportUnusedFunction]
        return HealthResponse()

    @app.get(
        "/readyz",
        response_model=ReadinessResponse,
        responses={503: {"model": ReadinessResponse}},
        tags=["system"],
    )
    async def _readyz(  # pyright: ignore[reportUnusedFunction]
        response: Response,
    ) -> ReadinessResponse:
        storage = MinioObjectStorage(
            active.minio_endpoint,
            active.minio_access_key,
            active.minio_secret_key,
            active.minio_bucket,
            secure=active.minio_secure,
            thread_limit=active.storage_thread_limit,
            operation_timeout_seconds=active.storage_operation_timeout_seconds,
        )
        checks: dict[str, tuple[bool, Check]] = {
            "postgresql": (True, database_ping),
            "redis": (False, lambda: redis_ping(active.redis_url)),
            "kafka": (True, lambda: kafka_ping(active.kafka_bootstrap_servers)),
            "minio": (False, storage.ensure_bucket),
        }
        dependencies: dict[str, DependencyStatus] = {}
        for name, (critical, check) in checks.items():
            try:
                await asyncio.wait_for(check(), timeout=active.infrastructure_timeout_seconds)
                dependencies[name] = DependencyStatus(critical=critical, status="available")
            except Exception:
                dependencies[name] = DependencyStatus(
                    critical=critical,
                    status="unavailable" if critical else "degraded",
                    reason=f"{name}_unavailable",
                )
        unavailable = any(
            item.critical and item.status != "available" for item in dependencies.values()
        )
        degraded = any(item.status != "available" for item in dependencies.values())
        status = "unavailable" if unavailable else "degraded" if degraded else "ready"
        if unavailable:
            response.status_code = 503
        return ReadinessResponse(status=status, dependencies=dependencies)

    @app.get("/metrics", include_in_schema=True, tags=["system"])
    async def _metrics() -> Response:  # pyright: ignore[reportUnusedFunction]
        return metrics_response()

    return app


app = create_app()

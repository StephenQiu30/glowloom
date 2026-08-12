from datetime import UTC, datetime, timedelta
from hashlib import sha256
from secrets import token_urlsafe
from uuid import UUID

import pytest
from pydantic import SecretStr
from sqlalchemy import text
from sqlalchemy.exc import IntegrityError
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker
from uuid6 import uuid7

from app.modules.identity.models import UserAccount, Workspace
from app.modules.production.models import ModelCapability
from app.modules.production.providers.contracts import CredentialIdentity
from app.modules.production.providers.credentials import AesGcmCredentialCipher
from app.modules.production.providers.models import (
    ProviderBinding,
    ProviderConnection,
    ProviderCredentialVersion,
    ProviderHealthCheck,
)


def _connection(*, workspace_id: UUID, owner_id: UUID) -> ProviderConnection:
    return ProviderConnection(
        id=uuid7(),
        workspace_id=workspace_id,
        preset_id="deepseek",
        catalog_version=1,
        display_name="DeepSeek production",
        protocol="openai_compatible",
        region=None,
        base_url="https://api.deepseek.com",
        non_secret_config={},
        configuration_status="valid",
        revision=1,
        created_by=owner_id,
        updated_by=owner_id,
    )


def _credential(
    *,
    workspace_id: UUID,
    connection_id: UUID,
    owner_id: UUID,
    version: int,
    status: str = "current",
) -> ProviderCredentialVersion:
    return ProviderCredentialVersion(
        id=uuid7(),
        workspace_id=workspace_id,
        connection_id=connection_id,
        version=version,
        key_id="provider-key-v1",
        nonce=b"n" * 12,
        ciphertext=f"ciphertext-{version}".encode(),
        auth_tag=b"t" * 16,
        fingerprint_hmac=f"{version:064x}",
        status=status,
        created_by=owner_id,
    )


async def _add_identity_fixture(
    session: AsyncSession,
) -> tuple[UUID, UUID, UUID]:
    owner_id = uuid7()
    workspace_id = uuid7()
    other_workspace_id = uuid7()
    session.add_all(
        (
            UserAccount(
                id=owner_id,
                email_normalized=f"provider-integrity-{owner_id}@example.com",
                password_hash="synthetic-not-used",
                display_name="Provider Integrity Fixture",
            ),
            Workspace(id=workspace_id, name="Provider Workspace"),
            Workspace(id=other_workspace_id, name="Foreign Provider Workspace"),
        )
    )
    await session.flush()
    return owner_id, workspace_id, other_workspace_id


@pytest.mark.asyncio
async def test_database_rejects_cross_workspace_credential_reference(
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    async with session_factory() as session:
        async with session.begin():
            owner_id, workspace_id, other_workspace_id = await _add_identity_fixture(session)
            connection = _connection(workspace_id=workspace_id, owner_id=owner_id)
            session.add(connection)
            await session.flush()

        with pytest.raises(IntegrityError):
            async with session.begin():
                session.add(
                    _credential(
                        workspace_id=other_workspace_id,
                        connection_id=connection.id,
                        owner_id=owner_id,
                        version=1,
                    )
                )
                await session.flush()


@pytest.mark.asyncio
async def test_database_allows_only_one_current_credential_per_connection(
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    async with session_factory() as session:
        async with session.begin():
            owner_id, workspace_id, _ = await _add_identity_fixture(session)
            connection = _connection(workspace_id=workspace_id, owner_id=owner_id)
            session.add(connection)
            await session.flush()
            session.add(
                _credential(
                    workspace_id=workspace_id,
                    connection_id=connection.id,
                    owner_id=owner_id,
                    version=1,
                )
            )

        with pytest.raises(IntegrityError):
            async with session.begin():
                session.add(
                    _credential(
                        workspace_id=workspace_id,
                        connection_id=connection.id,
                        owner_id=owner_id,
                        version=2,
                    )
                )
                await session.flush()


@pytest.mark.asyncio
async def test_database_allows_only_one_active_binding_per_workspace_usage(
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    async with session_factory() as session:
        async with session.begin():
            owner_id, workspace_id, _ = await _add_identity_fixture(session)
            connection = _connection(workspace_id=workspace_id, owner_id=owner_id)
            capability = ModelCapability(
                id=uuid7(),
                provider="deepseek",
                model="deepseek-v4-pro",
                kind="text",
                config_version=1,
                input_types=["text"],
                parameter_schema={},
                limits={},
                pricing=None,
                status="active",
            )
            session.add_all((connection, capability))
            await session.flush()
            credential = _credential(
                workspace_id=workspace_id,
                connection_id=connection.id,
                owner_id=owner_id,
                version=1,
            )
            session.add(credential)
            await session.flush()
            session.add(
                ProviderBinding(
                    workspace_id=workspace_id,
                    usage_type="script_structure",
                    connection_id=connection.id,
                    credential_version_id=credential.id,
                    capability_id=capability.id,
                    capability_config_version=capability.config_version,
                    binding_revision=1,
                    status="active",
                    activated_by=owner_id,
                    activated_at=datetime.now(UTC),
                )
            )

        with pytest.raises(IntegrityError):
            async with session.begin():
                session.add(
                    ProviderBinding(
                        workspace_id=workspace_id,
                        usage_type="script_structure",
                        connection_id=connection.id,
                        credential_version_id=credential.id,
                        capability_id=capability.id,
                        capability_config_version=capability.config_version,
                        binding_revision=2,
                        status="active",
                        activated_by=owner_id,
                        activated_at=datetime.now(UTC),
                    )
                )
                await session.flush()


@pytest.mark.asyncio
async def test_database_rejects_cross_workspace_binding_reference(
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    async with session_factory() as session:
        async with session.begin():
            owner_id, workspace_id, other_workspace_id = await _add_identity_fixture(session)
            connection = _connection(workspace_id=workspace_id, owner_id=owner_id)
            capability = ModelCapability(
                id=uuid7(),
                provider="deepseek",
                model="deepseek-v4-pro",
                kind="text",
                config_version=1,
                input_types=["text"],
                parameter_schema={},
                limits={},
                pricing=None,
                status="active",
            )
            session.add_all((connection, capability))
            await session.flush()
            credential = _credential(
                workspace_id=workspace_id,
                connection_id=connection.id,
                owner_id=owner_id,
                version=1,
            )
            session.add(credential)
            await session.flush()

        with pytest.raises(IntegrityError):
            async with session.begin():
                session.add(
                    ProviderBinding(
                        workspace_id=other_workspace_id,
                        usage_type="script_structure",
                        connection_id=connection.id,
                        credential_version_id=credential.id,
                        capability_id=capability.id,
                        capability_config_version=capability.config_version,
                        binding_revision=1,
                        status="active",
                        activated_by=owner_id,
                        activated_at=datetime.now(UTC),
                    )
                )
                await session.flush()


@pytest.mark.asyncio
async def test_database_rejects_binding_with_unregistered_capability_version(
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    async with session_factory() as session:
        async with session.begin():
            owner_id, workspace_id, _ = await _add_identity_fixture(session)
            connection = _connection(workspace_id=workspace_id, owner_id=owner_id)
            capability = ModelCapability(
                id=uuid7(),
                provider="deepseek",
                model="deepseek-v4-pro",
                kind="text",
                config_version=1,
                input_types=["text"],
                parameter_schema={},
                limits={},
                pricing=None,
                status="active",
            )
            session.add_all((connection, capability))
            await session.flush()
            credential = _credential(
                workspace_id=workspace_id,
                connection_id=connection.id,
                owner_id=owner_id,
                version=1,
            )
            session.add(credential)
            await session.flush()

        with pytest.raises(IntegrityError):
            async with session.begin():
                session.add(
                    ProviderBinding(
                        workspace_id=workspace_id,
                        usage_type="script_structure",
                        connection_id=connection.id,
                        credential_version_id=credential.id,
                        capability_id=capability.id,
                        capability_config_version=2,
                        binding_revision=1,
                        status="active",
                        activated_by=owner_id,
                        activated_at=datetime.now(UTC),
                    )
                )
                await session.flush()


@pytest.mark.asyncio
async def test_database_rejects_cross_workspace_health_check_reference(
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    async with session_factory() as session:
        async with session.begin():
            owner_id, workspace_id, other_workspace_id = await _add_identity_fixture(session)
            connection = _connection(workspace_id=workspace_id, owner_id=owner_id)
            session.add(connection)
            await session.flush()
            credential = _credential(
                workspace_id=workspace_id,
                connection_id=connection.id,
                owner_id=owner_id,
                version=1,
            )
            session.add(credential)
            await session.flush()

        with pytest.raises(IntegrityError):
            async with session.begin():
                session.add(
                    ProviderHealthCheck(
                        workspace_id=other_workspace_id,
                        connection_id=connection.id,
                        connection_revision=connection.revision,
                        credential_version_id=credential.id,
                        probe_type="model_discovery",
                        status="unreachable",
                        latency_ms=None,
                        safe_error_code="provider_unreachable",
                        checked_by=owner_id,
                        checked_at=datetime.now(UTC),
                        expires_at=datetime.now(UTC) + timedelta(minutes=5),
                    )
                )
                await session.flush()


@pytest.mark.asyncio
async def test_database_credential_columns_contain_no_plaintext_sentinel(
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    sentinel = token_urlsafe(32)
    cipher = AesGcmCredentialCipher(
        key_id="provider-key-v1",
        master_key=b"m" * 32,
        fingerprint_key=b"f" * 32,
    )

    async with session_factory() as session:
        async with session.begin():
            owner_id, workspace_id, _ = await _add_identity_fixture(session)
            connection = _connection(workspace_id=workspace_id, owner_id=owner_id)
            session.add(connection)
            await session.flush()
            credential_id = uuid7()
            encrypted = cipher.encrypt(
                identity=CredentialIdentity(
                    workspace_id=workspace_id,
                    connection_id=connection.id,
                    credential_version_id=credential_id,
                    version=1,
                ),
                credential=SecretStr(sentinel),
            )
            session.add(
                ProviderCredentialVersion(
                    id=credential_id,
                    workspace_id=workspace_id,
                    connection_id=connection.id,
                    version=1,
                    key_id=encrypted.key_id,
                    nonce=encrypted.nonce,
                    ciphertext=encrypted.ciphertext,
                    auth_tag=encrypted.auth_tag,
                    fingerprint_hmac=encrypted.fingerprint_hmac,
                    status="current",
                    created_by=owner_id,
                )
            )
            session.add(
                ProviderHealthCheck(
                    workspace_id=workspace_id,
                    connection_id=connection.id,
                    connection_revision=connection.revision,
                    credential_version_id=credential_id,
                    probe_type="model_discovery",
                    status="healthy",
                    latency_ms=42,
                    safe_error_code=None,
                    checked_by=owner_id,
                    checked_at=datetime.now(UTC),
                    expires_at=datetime.now(UTC) + timedelta(minutes=5),
                )
            )

        result = await session.execute(
            text(
                "SELECT key_id, encode(nonce, 'hex'), encode(ciphertext, 'hex'), "
                "encode(auth_tag, 'hex'), fingerprint_hmac "
                "FROM prod_provider_credential_versions"
            )
        )
        database_projection = repr(result.one())

    if sentinel in database_projection:
        sentinel_hash = sha256(sentinel.encode("utf-8")).hexdigest()[:12]
        raise AssertionError(f"plaintext sentinel detected: sha256={sentinel_hash}")


def test_provider_tables_expose_no_plaintext_or_probe_body_columns() -> None:
    forbidden = {"api_key", "credential", "plaintext", "request_body", "response_body"}

    for model in (
        ProviderConnection,
        ProviderCredentialVersion,
        ProviderBinding,
        ProviderHealthCheck,
    ):
        assert forbidden.isdisjoint(model.__table__.columns.keys())

import asyncio
import os
from collections.abc import AsyncIterator
from typing import cast
from uuid import uuid4

import pytest
from redis.asyncio import Redis

from app.core.config import Settings
from app.modules.identity.registration_verifications.contracts import (
    RegistrationVerificationUnavailableError,
)
from app.modules.identity.registration_verifications.crypto import (
    email_fingerprint,
    registration_code_digest,
    registration_ticket_digest,
    source_fingerprint,
)
from app.modules.identity.registration_verifications.redis_store import (
    RedisRegistrationVerificationStore,
)


@pytest.mark.skipif(
    os.getenv("LANVERSE_RUN_REDIS_CONTRACT") != "1",
    reason="set LANVERSE_RUN_REDIS_CONTRACT=1 with the configured Redis running",
)
@pytest.mark.asyncio
async def test_real_redis_registration_challenge_and_ticket_are_atomic_and_private() -> None:
    settings = Settings()
    environment = f"contract-registration-{uuid4().hex}"
    secret = "contract-registration-secret-with-at-least-32-bytes"
    store = RedisRegistrationVerificationStore(
        settings.redis_url,
        environment=environment,
    )
    observer = Redis.from_url(  # pyright: ignore[reportUnknownMemberType]
        settings.redis_url,
        decode_responses=False,
    )
    email = "creator@example.com"
    code = "123456"
    email_key = email_fingerprint(secret, email)
    source_key = source_fingerprint(secret, "127.0.0.1")
    code_digest = registration_code_digest(secret, email, code)
    pattern = f"lanverse:{environment}:identity:registration_verification:v1:*"
    try:
        reserved = await store.reserve_challenge(
            email_key=email_key,
            source_key=source_key,
            code_digest=code_digest,
            challenge_ttl_seconds=600,
            resend_seconds=60,
            max_attempts=5,
            source_window_seconds=3600,
            source_limit=20,
        )
        cooldown = await store.reserve_challenge(
            email_key=email_key,
            source_key=source_key,
            code_digest=code_digest,
            challenge_ttl_seconds=600,
            resend_seconds=60,
            max_attempts=5,
            source_window_seconds=3600,
            source_limit=20,
        )
        assert reserved.accepted
        assert not cooldown.accepted and cooldown.reason == "email_cooldown"

        scan = cast(
            AsyncIterator[bytes],
            observer.scan_iter(  # pyright: ignore[reportUnknownMemberType]
                match=pattern,
                count=100,
            ),
        )
        keys = [key async for key in scan]
        assert keys
        serialized = b"\n".join(keys)
        for key in keys:
            key_type = await observer.type(key)
            if key_type == b"hash":
                values = cast(
                    dict[bytes, bytes],
                    await observer.hgetall(key),
                )
                serialized += b"\n" + b"\n".join(values.keys())
                serialized += b"\n" + b"\n".join(values.values())
        assert code.encode() not in serialized
        assert email.encode() not in serialized

        wrong = await store.confirm_challenge(
            email_key=email_key,
            candidate_digest=registration_code_digest(secret, email, "000000"),
            ticket_digest=registration_ticket_digest(secret, "wrong-ticket"),
            email=email,
            ticket_ttl_seconds=600,
        )
        assert wrong.status == "invalid" and wrong.remaining_attempts == 4

        tickets = ("first-ticket", "second-ticket")
        confirmations = await asyncio.gather(
            *(
                store.confirm_challenge(
                    email_key=email_key,
                    candidate_digest=code_digest,
                    ticket_digest=registration_ticket_digest(secret, ticket),
                    email=email,
                    ticket_ttl_seconds=600,
                )
                for ticket in tickets
            )
        )
        assert sorted(result.status for result in confirmations) == [
            "confirmed",
            "expired",
        ]
        winner = tickets[
            confirmations.index(
                next(result for result in confirmations if result.status == "confirmed")
            )
        ]
        winner_digest = registration_ticket_digest(secret, winner)
        consumed = await asyncio.gather(
            store.consume_ticket(ticket_digest=winner_digest),
            store.consume_ticket(ticket_digest=winner_digest),
        )
        assert sorted(value is None for value in consumed) == [False, True]
        assert email in consumed
    finally:
        scan = cast(
            AsyncIterator[bytes],
            observer.scan_iter(  # pyright: ignore[reportUnknownMemberType]
                match=pattern,
                count=100,
            ),
        )
        keys = [key async for key in scan]
        if keys:
            await observer.delete(*keys)
        await observer.aclose()
        await store.close()

    unavailable = RedisRegistrationVerificationStore(
        "redis://127.0.0.1:1/0",
        environment=environment,
    )
    try:
        with pytest.raises(RegistrationVerificationUnavailableError):
            await unavailable.reserve_challenge(
                email_key=email_key,
                source_key=source_key,
                code_digest=code_digest,
                challenge_ttl_seconds=600,
                resend_seconds=60,
                max_attempts=5,
                source_window_seconds=3600,
                source_limit=20,
            )
    finally:
        await unavailable.close()

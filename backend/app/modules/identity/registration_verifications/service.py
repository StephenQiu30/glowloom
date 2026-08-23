from collections.abc import Callable

from sqlalchemy.ext.asyncio import AsyncSession

from app.core.config import Settings
from app.core.errors import ApiError, ErrorCode
from app.modules.identity import repository
from app.modules.identity.registration_verifications.contracts import (
    RegistrationMailer,
    RegistrationMailUnavailableError,
    RegistrationVerificationStore,
    RegistrationVerificationUnavailableError,
)
from app.modules.identity.registration_verifications.crypto import (
    email_fingerprint,
    generate_registration_ticket,
    normalize_email,
    registration_code_digest,
    registration_ticket_digest,
    source_fingerprint,
)
from app.modules.identity.registration_verifications.schemas import (
    RegistrationVerificationAccepted,
    RegistrationVerificationConfirmed,
    RegistrationVerificationConfirmRequest,
    RegistrationVerificationRequest,
)


def _secret(settings: Settings) -> str:
    return settings.email_verification_hmac_secret.get_secret_value()


def _dependency_unavailable() -> ApiError:
    return ApiError(
        ErrorCode.DEPENDENCY_UNAVAILABLE,
        "Registration verification is temporarily unavailable",
        status_code=503,
        next_action="retry_registration_verification",
    )


async def request_verification(
    session: AsyncSession,
    request: RegistrationVerificationRequest,
    settings: Settings,
    store: RegistrationVerificationStore,
    mailer: RegistrationMailer,
    *,
    source: str,
    generate_code: Callable[[], str],
) -> RegistrationVerificationAccepted:
    email = normalize_email(str(request.email))
    secret = _secret(settings)
    code = generate_code()
    code_digest = registration_code_digest(secret, email, code)
    email_key = email_fingerprint(secret, email)
    source_key = source_fingerprint(secret, source)
    try:
        reservation = await store.reserve_challenge(
            email_key=email_key,
            source_key=source_key,
            code_digest=code_digest,
            challenge_ttl_seconds=settings.email_verification_ttl_seconds,
            resend_seconds=settings.email_verification_resend_seconds,
            max_attempts=settings.email_verification_max_attempts,
            source_window_seconds=settings.email_verification_source_window_seconds,
            source_limit=settings.email_verification_source_limit,
        )
    except RegistrationVerificationUnavailableError as error:
        raise _dependency_unavailable() from error
    if not reservation.accepted:
        raise ApiError(
            ErrorCode.RATE_LIMITED,
            "Registration verification requests are temporarily limited",
            status_code=429,
            next_action="retry_after_cooldown",
            details={"retry_after_seconds": reservation.retry_after_seconds},
        )

    existing = await repository.find_user_by_email(session, email)
    if existing is not None:
        try:
            await store.discard_challenge(email_key=email_key, code_digest=code_digest)
        except RegistrationVerificationUnavailableError as error:
            raise _dependency_unavailable() from error
        return RegistrationVerificationAccepted(
            email_sent=False, retry_after_seconds=reservation.retry_after_seconds
        )

    try:
        await mailer.send_registration_code(
            email=email,
            code=code,
            expires_minutes=max(1, settings.email_verification_ttl_seconds // 60),
        )
    except (RegistrationMailUnavailableError, ConnectionError) as error:
        try:
            await store.abandon_challenge(
                email_key=email_key,
                code_digest=code_digest,
            )
        except RegistrationVerificationUnavailableError:
            pass
        raise _dependency_unavailable() from error
    return RegistrationVerificationAccepted(
        email_sent=True, retry_after_seconds=reservation.retry_after_seconds
    )


async def confirm_verification(
    request: RegistrationVerificationConfirmRequest,
    settings: Settings,
    store: RegistrationVerificationStore,
) -> RegistrationVerificationConfirmed:
    email = normalize_email(str(request.email))
    secret = _secret(settings)
    ticket = generate_registration_ticket()
    try:
        result = await store.confirm_challenge(
            email_key=email_fingerprint(secret, email),
            candidate_digest=registration_code_digest(secret, email, request.code),
            ticket_digest=registration_ticket_digest(secret, ticket),
            email=email,
            ticket_ttl_seconds=settings.email_verification_ticket_ttl_seconds,
        )
    except RegistrationVerificationUnavailableError as error:
        raise _dependency_unavailable() from error
    if result.status == "invalid":
        raise ApiError(
            ErrorCode.INVALID_VERIFICATION_CODE,
            "Registration verification code is invalid",
            status_code=422,
            next_action="retry_verification_code",
            details={"remaining_attempts": result.remaining_attempts},
        )
    if result.status == "expired":
        raise ApiError(
            ErrorCode.VERIFICATION_EXPIRED,
            "Registration verification has expired",
            status_code=410,
            next_action="request_registration_verification",
        )
    return RegistrationVerificationConfirmed(
        registration_ticket=ticket,
        expires_in=settings.email_verification_ticket_ttl_seconds,
    )


async def consume_registration_ticket(
    ticket: str,
    settings: Settings,
    store: RegistrationVerificationStore,
) -> str:
    try:
        email = await store.consume_ticket(
            ticket_digest=registration_ticket_digest(_secret(settings), ticket)
        )
    except RegistrationVerificationUnavailableError as error:
        raise _dependency_unavailable() from error
    if email is None:
        raise ApiError(
            ErrorCode.VERIFICATION_EXPIRED,
            "Registration verification has expired",
            status_code=410,
            next_action="request_registration_verification",
        )
    return email

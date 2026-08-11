import hmac
from dataclasses import dataclass

from app.modules.identity.registration_verifications.contracts import (
    ChallengeReservation,
    ConfirmationResult,
    RegistrationMailUnavailableError,
    RegistrationVerificationStore,
    RegistrationVerificationUnavailableError,
)


@dataclass
class _Challenge:
    digest: str
    remaining_attempts: int


class MemoryRegistrationVerificationStore(RegistrationVerificationStore):
    def __init__(self) -> None:
        self.challenges: dict[str, _Challenge] = {}
        self.tickets: dict[str, str] = {}
        self.unavailable = False

    async def reserve_challenge(
        self,
        *,
        email_key: str,
        source_key: str,
        code_digest: str,
        challenge_ttl_seconds: int,
        resend_seconds: int,
        max_attempts: int,
        source_window_seconds: int,
        source_limit: int,
    ) -> ChallengeReservation:
        self._ensure_available()
        self.challenges[email_key] = _Challenge(code_digest, max_attempts)
        return ChallengeReservation(accepted=True, retry_after_seconds=resend_seconds)

    async def abandon_challenge(self, *, email_key: str, code_digest: str) -> None:
        self._ensure_available()
        challenge = self.challenges.get(email_key)
        if challenge is not None and hmac.compare_digest(challenge.digest, code_digest):
            self.challenges.pop(email_key, None)

    async def discard_challenge(self, *, email_key: str, code_digest: str) -> None:
        await self.abandon_challenge(email_key=email_key, code_digest=code_digest)

    async def confirm_challenge(
        self,
        *,
        email_key: str,
        candidate_digest: str,
        ticket_digest: str,
        email: str,
        ticket_ttl_seconds: int,
    ) -> ConfirmationResult:
        self._ensure_available()
        challenge = self.challenges.get(email_key)
        if challenge is None:
            return ConfirmationResult(status="expired", remaining_attempts=0)
        if not hmac.compare_digest(challenge.digest, candidate_digest):
            challenge.remaining_attempts -= 1
            if challenge.remaining_attempts <= 0:
                self.challenges.pop(email_key, None)
                return ConfirmationResult(status="expired", remaining_attempts=0)
            return ConfirmationResult(
                status="invalid",
                remaining_attempts=challenge.remaining_attempts,
            )
        self.challenges.pop(email_key, None)
        self.tickets[ticket_digest] = email
        return ConfirmationResult(status="confirmed", remaining_attempts=0)

    async def consume_ticket(self, *, ticket_digest: str) -> str | None:
        self._ensure_available()
        return self.tickets.pop(ticket_digest, None)

    async def close(self) -> None:
        return None

    def _ensure_available(self) -> None:
        if self.unavailable:
            raise RegistrationVerificationUnavailableError(
                "registration verification dependency is unavailable"
            )


class RecordingRegistrationMailer:
    def __init__(self) -> None:
        self.messages: list[tuple[str, str, int]] = []
        self.unavailable = False

    async def send_registration_code(
        self,
        *,
        email: str,
        code: str,
        expires_minutes: int,
    ) -> None:
        if self.unavailable:
            raise RegistrationMailUnavailableError(
                "registration mail dependency is unavailable"
            )
        self.messages.append((email, code, expires_minutes))

from dataclasses import dataclass
from typing import Literal, Protocol


class RegistrationVerificationUnavailableError(RuntimeError):
    pass


class RegistrationMailUnavailableError(RuntimeError):
    pass


ReservationReason = Literal["accepted", "email_cooldown", "source_limit"]
ConfirmationStatus = Literal["confirmed", "invalid", "expired"]


@dataclass(frozen=True, slots=True)
class ChallengeReservation:
    accepted: bool
    retry_after_seconds: int
    reason: ReservationReason = "accepted"


@dataclass(frozen=True, slots=True)
class ConfirmationResult:
    status: ConfirmationStatus
    remaining_attempts: int


class RegistrationVerificationStore(Protocol):
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
    ) -> ChallengeReservation: ...

    async def discard_challenge(
        self,
        *,
        email_key: str,
        code_digest: str,
    ) -> None: ...
    async def abandon_challenge(
        self,
        *,
        email_key: str,
        code_digest: str,
    ) -> None: ...

    async def confirm_challenge(
        self,
        *,
        email_key: str,
        candidate_digest: str,
        ticket_digest: str,
        email: str,
        ticket_ttl_seconds: int,
    ) -> ConfirmationResult: ...

    async def consume_ticket(self, *, ticket_digest: str) -> str | None: ...

    async def close(self) -> None: ...


class RegistrationMailer(Protocol):
    async def send_registration_code(
        self,
        *,
        email: str,
        code: str,
        expires_minutes: int,
    ) -> None: ...

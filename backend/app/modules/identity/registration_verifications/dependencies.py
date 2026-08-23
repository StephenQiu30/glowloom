from collections.abc import Callable
from typing import cast

from fastapi import Request

from app.modules.identity.registration_verifications.contracts import (
    RegistrationMailer,
    RegistrationVerificationStore,
)


def get_registration_verification_store(
    request: Request,
) -> RegistrationVerificationStore:
    return cast(
        RegistrationVerificationStore,
        request.app.state.registration_verification_store,
    )


def get_registration_mailer(request: Request) -> RegistrationMailer:
    return cast(RegistrationMailer, request.app.state.registration_mailer)


def get_registration_code_generator(request: Request) -> Callable[[], str]:
    return cast(Callable[[], str], request.app.state.registration_code_generator)

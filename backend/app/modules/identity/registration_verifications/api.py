from collections.abc import Callable
from typing import Annotated

from fastapi import APIRouter, Depends, Request, status
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.auth import get_request_settings
from app.core.config import Settings
from app.core.database import get_async_session
from app.core.schemas import ApiResponse
from app.modules.identity.registration_verifications import service
from app.modules.identity.registration_verifications.contracts import (
    RegistrationMailer,
    RegistrationVerificationStore,
)
from app.modules.identity.registration_verifications.dependencies import (
    get_registration_code_generator,
    get_registration_mailer,
    get_registration_verification_store,
)
from app.modules.identity.registration_verifications.schemas import (
    RegistrationVerificationAccepted,
    RegistrationVerificationConfirmed,
    RegistrationVerificationConfirmRequest,
    RegistrationVerificationRequest,
)

router = APIRouter(prefix="/api/v1", tags=["identity"])


@router.post(
    "/auth/registration-verifications",
    response_model=ApiResponse[RegistrationVerificationAccepted],
    status_code=status.HTTP_202_ACCEPTED,
)
async def request_registration_verification(
    payload: RegistrationVerificationRequest,
    request: Request,
    session: Annotated[AsyncSession, Depends(get_async_session)],
    settings: Annotated[Settings, Depends(get_request_settings)],
    store: Annotated[
        RegistrationVerificationStore,
        Depends(get_registration_verification_store),
    ],
    mailer: Annotated[RegistrationMailer, Depends(get_registration_mailer)],
    generate_code: Annotated[
        Callable[[], str],
        Depends(get_registration_code_generator),
    ],
) -> ApiResponse[RegistrationVerificationAccepted]:
    source = request.client.host if request.client is not None else "unknown"
    return ApiResponse(
        data=await service.request_verification(
            session,
            payload,
            settings,
            store,
            mailer,
            source=source,
            generate_code=generate_code,
        )
    )


@router.post(
    "/auth/registration-verifications/confirm",
    response_model=ApiResponse[RegistrationVerificationConfirmed],
)
async def confirm_registration_verification(
    payload: RegistrationVerificationConfirmRequest,
    settings: Annotated[Settings, Depends(get_request_settings)],
    store: Annotated[
        RegistrationVerificationStore,
        Depends(get_registration_verification_store),
    ],
) -> ApiResponse[RegistrationVerificationConfirmed]:
    return ApiResponse(
        data=await service.confirm_verification(payload, settings, store)
    )

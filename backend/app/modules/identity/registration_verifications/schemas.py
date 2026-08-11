from typing import Literal

from pydantic import BaseModel, EmailStr, Field

from app.modules.identity.authentication.schemas import CommandModel


class RegistrationVerificationRequest(CommandModel):
    email: EmailStr


class RegistrationVerificationConfirmRequest(CommandModel):
    email: EmailStr
    code: str = Field(pattern=r"^\d{6}$")


class RegistrationVerificationAccepted(BaseModel):
    accepted: Literal[True] = True
    retry_after_seconds: int


class RegistrationVerificationConfirmed(BaseModel):
    registration_ticket: str
    expires_in: int

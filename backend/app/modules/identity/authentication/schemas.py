from typing import Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, EmailStr, Field, HttpUrl, SecretStr

from app.modules.identity.workspaces.schemas import WorkspaceResponse


class CommandModel(BaseModel):
    model_config = ConfigDict(extra="forbid")


class RegisterRequest(CommandModel):
    email: EmailStr
    password: SecretStr
    display_name: str = Field(min_length=1, max_length=80)


class LoginRequest(CommandModel):
    email: EmailStr
    password: SecretStr


class ChangePasswordRequest(CommandModel):
    current_password: SecretStr
    new_password: SecretStr


class UserResponse(BaseModel):
    model_config = ConfigDict(from_attributes=True)

    id: UUID
    email: EmailStr
    display_name: str
    avatar_url: str | None


class AuthResponse(BaseModel):
    user: UserResponse
    workspace: WorkspaceResponse
    access_token: str
    token_type: Literal["bearer"] = "bearer"
    expires_in: int


class MeResponse(BaseModel):
    user: UserResponse
    workspace: WorkspaceResponse


class RevocationResponse(BaseModel):
    revoked: Literal[True] = True


class ProfileUpdateRequest(CommandModel):
    display_name: str | None = Field(default=None, min_length=1, max_length=80)
    avatar_url: HttpUrl | None = None


class DeactivateAccountRequest(CommandModel):
    confirmation: Literal["DEACTIVATE"]

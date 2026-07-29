from typing import Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, EmailStr, Field, HttpUrl, SecretStr


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


class WorkspaceResponse(BaseModel):
    id: UUID
    name: str
    status: Literal["active", "archived"]
    role: Literal["owner", "editor", "viewer"]
    revision: int


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


class WorkspaceCreateRequest(CommandModel):
    name: str = Field(min_length=1, max_length=120)


class WorkspaceUpdateRequest(CommandModel):
    name: str = Field(min_length=1, max_length=120)
    expected_revision: int = Field(ge=1)


class WorkspaceStateRequest(CommandModel):
    expected_revision: int = Field(ge=1)


class DeactivateAccountRequest(CommandModel):
    confirmation: Literal["DEACTIVATE"]

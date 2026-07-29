from typing import Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, EmailStr, Field, SecretStr


class RegisterRequest(BaseModel):
    email: EmailStr
    password: SecretStr
    display_name: str = Field(min_length=1, max_length=80)


class LoginRequest(BaseModel):
    email: EmailStr
    password: SecretStr


class ChangePasswordRequest(BaseModel):
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

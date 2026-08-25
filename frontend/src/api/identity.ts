import request, { type RequestOptions } from "@/lib/request";

type Envelope<T> = { data: T };

export function requestRegistrationVerificationApiV1AuthRegistrationVerificationsPost(
  body: API.RegistrationVerificationRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.RegistrationVerificationAccepted>>(
    "/api/v1/auth/registration-verifications",
    { method: "POST", data: body, ...(options ?? {}) },
  );
}

export function confirmRegistrationVerificationApiV1AuthRegistrationVerificationsConfirmPost(
  body: API.RegistrationVerificationConfirmRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.RegistrationVerificationConfirmed>>(
    "/api/v1/auth/registration-verifications/confirm",
    { method: "POST", data: body, ...(options ?? {}) },
  );
}

export function registerApiV1AuthRegisterPost(
  body: API.RegisterRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.AuthResponse>>("/api/v1/auth/register", {
    method: "POST",
    data: body,
    ...(options ?? {}),
  });
}

export function loginApiV1AuthLoginPost(
  body: API.LoginRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.AuthResponse>>("/api/v1/auth/login", {
    method: "POST",
    data: body,
    ...(options ?? {}),
  });
}

export function logoutApiV1AuthLogoutPost(options?: RequestOptions) {
  return request<Envelope<API.RevocationResponse>>("/api/v1/auth/logout", {
    method: "POST",
    ...(options ?? {}),
  });
}

export function changePasswordApiV1AuthChangePasswordPost(
  body: API.ChangePasswordRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.RevocationResponse>>(
    "/api/v1/auth/change-password",
    { method: "POST", data: body, ...(options ?? {}) },
  );
}

export function meApiV1MeGet(options?: RequestOptions) {
  return request<Envelope<API.MeResponse>>("/api/v1/me", {
    method: "GET",
    ...(options ?? {}),
  });
}

export function updateMeApiV1MePatch(
  body: API.ProfileUpdateRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.MeResponse>>("/api/v1/me", {
    method: "PATCH",
    data: body,
    ...(options ?? {}),
  });
}

export function deactivateMeApiV1MeDeactivatePost(
  body: API.DeactivateAccountRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.RevocationResponse>>("/api/v1/me/deactivate", {
    method: "POST",
    data: body,
    ...(options ?? {}),
  });
}

export function listWorkspacesApiV1WorkspacesGet(
  params: { include_archived: boolean },
  options?: RequestOptions,
) {
  return request<Envelope<API.WorkspaceResponse[]>>("/api/v1/workspaces", {
    method: "GET",
    params,
    ...(options ?? {}),
  });
}

export function createWorkspaceApiV1WorkspacesPost(
  body: API.WorkspaceCreateRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.WorkspaceResponse>>("/api/v1/workspaces", {
    method: "POST",
    data: body,
    ...(options ?? {}),
  });
}

export function updateWorkspaceApiV1WorkspacesWorkspaceIdPatch(
  params: { workspace_id: string },
  body: API.WorkspaceUpdateRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.WorkspaceResponse>>(
    `/api/v1/workspaces/${params.workspace_id}`,
    { method: "PATCH", data: body, ...(options ?? {}) },
  );
}

export function archiveWorkspaceApiV1WorkspacesWorkspaceIdArchivePost(
  params: { workspace_id: string },
  body: API.WorkspaceStateRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.WorkspaceResponse>>(
    `/api/v1/workspaces/${params.workspace_id}/archive`,
    { method: "POST", data: body, ...(options ?? {}) },
  );
}

export function restoreWorkspaceApiV1WorkspacesWorkspaceIdRestorePost(
  params: { workspace_id: string },
  body: API.WorkspaceStateRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.WorkspaceResponse>>(
    `/api/v1/workspaces/${params.workspace_id}/restore`,
    { method: "POST", data: body, ...(options ?? {}) },
  );
}

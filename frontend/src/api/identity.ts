import request, { type RequestOptions } from "@/lib/request";

type Envelope<T> = { data: T };

export function requestRegistrationVerificationApiAuthRegistrationVerificationsPost(
  body: API.RegistrationVerificationRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.RegistrationVerificationAccepted>>(
    "/api/auth/registration-verifications",
    { method: "POST", data: body, ...(options ?? {}) },
  );
}

export function confirmRegistrationVerificationApiAuthRegistrationVerificationsConfirmPost(
  body: API.RegistrationVerificationConfirmRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.RegistrationVerificationConfirmed>>(
    "/api/auth/registration-verifications/confirm",
    { method: "POST", data: body, ...(options ?? {}) },
  );
}

export function registerApiAuthRegisterPost(
  body: API.RegisterRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.AuthResponse>>("/api/auth/register", {
    method: "POST",
    data: body,
    ...(options ?? {}),
  });
}

export function loginApiAuthLoginPost(
  body: API.LoginRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.AuthResponse>>("/api/auth/login", {
    method: "POST",
    data: body,
    ...(options ?? {}),
  });
}

export function logoutApiAuthLogoutPost(options?: RequestOptions) {
  return request<Envelope<API.RevocationResponse>>("/api/auth/logout", {
    method: "POST",
    ...(options ?? {}),
  });
}

export function changePasswordApiAuthChangePasswordPost(
  body: API.ChangePasswordRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.RevocationResponse>>(
    "/api/auth/change-password",
    { method: "POST", data: body, ...(options ?? {}) },
  );
}

export function meApiMeGet(options?: RequestOptions) {
  return request<Envelope<API.MeResponse>>("/api/me", {
    method: "GET",
    ...(options ?? {}),
  });
}

export function updateMeApiMePatch(
  body: API.ProfileUpdateRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.MeResponse>>("/api/me", {
    method: "PATCH",
    data: body,
    ...(options ?? {}),
  });
}

export function deactivateMeApiMeDeactivatePost(
  body: API.DeactivateAccountRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.RevocationResponse>>("/api/me/deactivate", {
    method: "POST",
    data: body,
    ...(options ?? {}),
  });
}

export function listWorkspacesApiWorkspacesGet(
  params: { include_archived: boolean },
  options?: RequestOptions,
) {
  return request<Envelope<API.WorkspaceResponse[]>>("/api/workspaces", {
    method: "GET",
    params,
    ...(options ?? {}),
  });
}

export function createWorkspaceApiWorkspacesPost(
  body: API.WorkspaceCreateRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.WorkspaceResponse>>("/api/workspaces", {
    method: "POST",
    data: body,
    ...(options ?? {}),
  });
}

export function updateWorkspaceApiWorkspacesWorkspaceIdPatch(
  params: { workspace_id: string },
  body: API.WorkspaceUpdateRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.WorkspaceResponse>>(
    `/api/workspaces/${params.workspace_id}`,
    { method: "PATCH", data: body, ...(options ?? {}) },
  );
}

export function archiveWorkspaceApiWorkspacesWorkspaceIdArchivePost(
  params: { workspace_id: string },
  body: API.WorkspaceStateRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.WorkspaceResponse>>(
    `/api/workspaces/${params.workspace_id}/archive`,
    { method: "POST", data: body, ...(options ?? {}) },
  );
}

export function restoreWorkspaceApiWorkspacesWorkspaceIdRestorePost(
  params: { workspace_id: string },
  body: API.WorkspaceStateRequest,
  options?: RequestOptions,
) {
  return request<Envelope<API.WorkspaceResponse>>(
    `/api/workspaces/${params.workspace_id}/restore`,
    { method: "POST", data: body, ...(options ?? {}) },
  );
}

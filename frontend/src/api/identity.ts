import request, { type RequestOptions } from "@/lib/request";

/** Change Password POST /api/v1/auth/change-password */
export async function changePasswordApiV1AuthChangePasswordPost(
  body: API.ChangePasswordRequest,
  options?: RequestOptions,
) {
  return request<API.ApiResponseRevocationResponse_>(`/api/v1/auth/change-password`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Login POST /api/v1/auth/login */
export async function loginApiV1AuthLoginPost(
  body: API.LoginRequest,
  options?: RequestOptions,
) {
  return request<API.ApiResponseAuthResponse_>(`/api/v1/auth/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Logout POST /api/v1/auth/logout */
export async function logoutApiV1AuthLogoutPost(
  options?: RequestOptions,
) {
  return request<API.ApiResponseRevocationResponse_>(`/api/v1/auth/logout`, {
    method: "POST",
    ...(options ?? {}),
  });
}

/** Refresh POST /api/v1/auth/refresh */
export async function refreshApiV1AuthRefreshPost(
  options?: RequestOptions,
) {
  return request<API.ApiResponseAuthResponse_>(`/api/v1/auth/refresh`, {
    method: "POST",
    ...(options ?? {}),
  });
}

/** Register POST /api/v1/auth/register */
export async function registerApiV1AuthRegisterPost(
  body: API.RegisterRequest,
  options?: RequestOptions,
) {
  return request<API.ApiResponseAuthResponse_>(`/api/v1/auth/register`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Request Registration Verification POST /api/v1/auth/registration-verifications */
export async function requestRegistrationVerificationApiV1AuthRegistrationVerificationsPost(
  body: API.RegistrationVerificationRequest,
  options?: RequestOptions,
) {
  return request<API.ApiResponseRegistrationVerificationAccepted_>(`/api/v1/auth/registration-verifications`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Confirm Registration Verification POST /api/v1/auth/registration-verifications/confirm */
export async function confirmRegistrationVerificationApiV1AuthRegistrationVerificationsConfirmPost(
  body: API.RegistrationVerificationConfirmRequest,
  options?: RequestOptions,
) {
  return request<API.ApiResponseRegistrationVerificationConfirmed_>(`/api/v1/auth/registration-verifications/confirm`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Me GET /api/v1/me */
export async function meApiV1MeGet(
  options?: RequestOptions,
) {
  return request<API.ApiResponseMeResponse_>(`/api/v1/me`, {
    method: "GET",
    ...(options ?? {}),
  });
}

/** Update Me PATCH /api/v1/me */
export async function updateMeApiV1MePatch(
  body: API.ProfileUpdateRequest,
  options?: RequestOptions,
) {
  return request<API.ApiResponseMeResponse_>(`/api/v1/me`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Deactivate Me POST /api/v1/me/deactivate */
export async function deactivateMeApiV1MeDeactivatePost(
  body: API.DeactivateAccountRequest,
  options?: RequestOptions,
) {
  return request<API.ApiResponseRevocationResponse_>(`/api/v1/me/deactivate`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** List Workspaces GET /api/v1/workspaces */
export async function listWorkspacesApiV1WorkspacesGet(
  params: API.listWorkspacesApiV1WorkspacesGetParams,
  options?: RequestOptions,
) {
  return request<API.ApiResponseListWorkspaceResponse_>(`/api/v1/workspaces`, {
    method: "GET",
    params,
    ...(options ?? {}),
  });
}

/** Create Workspace POST /api/v1/workspaces */
export async function createWorkspaceApiV1WorkspacesPost(
  body: API.WorkspaceCreateRequest,
  options?: RequestOptions,
) {
  return request<API.ApiResponseWorkspaceResponse_>(`/api/v1/workspaces`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Get Workspace GET /api/v1/workspaces/{workspace_id} */
export async function getWorkspaceApiV1WorkspacesWorkspaceIdGet(
  params: API.getWorkspaceApiV1WorkspacesWorkspaceIdGetParams,
  options?: RequestOptions,
) {
  const { workspace_id: path0 } = params;
  return request<API.ApiResponseWorkspaceResponse_>(`/api/v1/workspaces/${path0}`, {
    method: "GET",
    ...(options ?? {}),
  });
}

/** Update Workspace PATCH /api/v1/workspaces/{workspace_id} */
export async function updateWorkspaceApiV1WorkspacesWorkspaceIdPatch(
  params: API.updateWorkspaceApiV1WorkspacesWorkspaceIdPatchParams,
  body: API.WorkspaceUpdateRequest,
  options?: RequestOptions,
) {
  const { workspace_id: path0 } = params;
  return request<API.ApiResponseWorkspaceResponse_>(`/api/v1/workspaces/${path0}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Archive Workspace POST /api/v1/workspaces/{workspace_id}/archive */
export async function archiveWorkspaceApiV1WorkspacesWorkspaceIdArchivePost(
  params: API.archiveWorkspaceApiV1WorkspacesWorkspaceIdArchivePostParams,
  body: API.WorkspaceStateRequest,
  options?: RequestOptions,
) {
  const { workspace_id: path0 } = params;
  return request<API.ApiResponseWorkspaceResponse_>(`/api/v1/workspaces/${path0}/archive`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Restore Workspace POST /api/v1/workspaces/{workspace_id}/restore */
export async function restoreWorkspaceApiV1WorkspacesWorkspaceIdRestorePost(
  params: API.restoreWorkspaceApiV1WorkspacesWorkspaceIdRestorePostParams,
  body: API.WorkspaceStateRequest,
  options?: RequestOptions,
) {
  const { workspace_id: path0 } = params;
  return request<API.ApiResponseWorkspaceResponse_>(`/api/v1/workspaces/${path0}/restore`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

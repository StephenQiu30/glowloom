// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/api-request";

/** Change Password POST /api/v1/auth/change-password */
export async function changePasswordApiV1AuthChangePasswordPost(
  body: API.ChangePasswordRequest,
  options?: RequestOptions
) {
  return request<API.ApiResponseRevocationResponse_>(
    "/api/v1/auth/change-password",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      data: body,
      ...(options || {}),
    }
  );
}

/** Login POST /api/v1/auth/login */
export async function loginApiV1AuthLoginPost(
  body: API.LoginRequest,
  options?: RequestOptions
) {
  return request<API.ApiResponseAuthResponse_>("/api/v1/auth/login", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** Logout POST /api/v1/auth/logout */
export async function logoutApiV1AuthLogoutPost(options?: RequestOptions) {
  return request<API.ApiResponseRevocationResponse_>("/api/v1/auth/logout", {
    method: "POST",
    ...(options || {}),
  });
}

/** Register POST /api/v1/auth/register */
export async function registerApiV1AuthRegisterPost(
  body: API.RegisterRequest,
  options?: RequestOptions
) {
  return request<API.ApiResponseAuthResponse_>("/api/v1/auth/register", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** Me GET /api/v1/me */
export async function meApiV1MeGet(options?: RequestOptions) {
  return request<API.ApiResponseMeResponse_>("/api/v1/me", {
    method: "GET",
    ...(options || {}),
  });
}

/** Update Me PATCH /api/v1/me */
export async function updateMeApiV1MePatch(
  body: API.ProfileUpdateRequest,
  options?: RequestOptions
) {
  return request<API.ApiResponseMeResponse_>("/api/v1/me", {
    method: "PATCH",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** Deactivate Me POST /api/v1/me/deactivate */
export async function deactivateMeApiV1MeDeactivatePost(
  body: API.DeactivateAccountRequest,
  options?: RequestOptions
) {
  return request<API.ApiResponseRevocationResponse_>("/api/v1/me/deactivate", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** List Workspaces GET /api/v1/workspaces */
export async function listWorkspacesApiV1WorkspacesGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listWorkspacesApiV1WorkspacesGetParams,
  options?: RequestOptions
) {
  return request<API.ApiResponseListWorkspaceResponse_>("/api/v1/workspaces", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** Create Workspace POST /api/v1/workspaces */
export async function createWorkspaceApiV1WorkspacesPost(
  body: API.WorkspaceCreateRequest,
  options?: RequestOptions
) {
  return request<API.ApiResponseWorkspaceResponse_>("/api/v1/workspaces", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** Get Workspace GET /api/v1/workspaces/${param0} */
export async function getWorkspaceApiV1WorkspacesWorkspaceIdGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getWorkspaceApiV1WorkspacesWorkspaceIdGetParams,
  options?: RequestOptions
) {
  const { workspace_id: param0, ...queryParams } = params;
  return request<API.ApiResponseWorkspaceResponse_>(
    `/api/v1/workspaces/${param0}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Update Workspace PATCH /api/v1/workspaces/${param0} */
export async function updateWorkspaceApiV1WorkspacesWorkspaceIdPatch(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.updateWorkspaceApiV1WorkspacesWorkspaceIdPatchParams,
  body: API.WorkspaceUpdateRequest,
  options?: RequestOptions
) {
  const { workspace_id: param0, ...queryParams } = params;
  return request<API.ApiResponseWorkspaceResponse_>(
    `/api/v1/workspaces/${param0}`,
    {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    }
  );
}

/** Archive Workspace POST /api/v1/workspaces/${param0}/archive */
export async function archiveWorkspaceApiV1WorkspacesWorkspaceIdArchivePost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.archiveWorkspaceApiV1WorkspacesWorkspaceIdArchivePostParams,
  body: API.WorkspaceStateRequest,
  options?: RequestOptions
) {
  const { workspace_id: param0, ...queryParams } = params;
  return request<API.ApiResponseWorkspaceResponse_>(
    `/api/v1/workspaces/${param0}/archive`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    }
  );
}

/** Restore Workspace POST /api/v1/workspaces/${param0}/restore */
export async function restoreWorkspaceApiV1WorkspacesWorkspaceIdRestorePost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.restoreWorkspaceApiV1WorkspacesWorkspaceIdRestorePostParams,
  body: API.WorkspaceStateRequest,
  options?: RequestOptions
) {
  const { workspace_id: param0, ...queryParams } = params;
  return request<API.ApiResponseWorkspaceResponse_>(
    `/api/v1/workspaces/${param0}/restore`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    }
  );
}

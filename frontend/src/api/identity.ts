// @ts-ignore
/* eslint-disable */
import { request } from "@/lib/request";
import type { RequestOptions } from "@/lib/request";

/** Check login availability before registration GET /api/accounts/availability */
export async function getLoginAvailability(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getLoginAvailabilityParams,
  options?: RequestOptions
) {
  return request<API.AvailabilityView>("/api/accounts/availability", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** Register a creator and establish a session POST /api/accounts/register */
export async function registerAccount(
  body: API.RegisterRequest,
  options?: RequestOptions
) {
  return request<API.SessionView>("/api/accounts/register", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/admin/users */
export async function listAdminUsers(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listAdminUsersParams,
  options?: RequestOptions
) {
  return request<API.AdminUserPage>("/api/admin/users", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/admin/users */
export async function createAdminUser(
  body: API.AdminCreateRequest,
  options?: RequestOptions
) {
  return request<API.AdminMutationReceipt>("/api/admin/users", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 PUT /api/admin/users/${param0} */
export async function updateAdminUser(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.updateAdminUserParams,
  body: API.AdminUpdateRequest,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<API.AdminMutationReceipt>(`/api/admin/users/${param0}`, {
    method: "PUT",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/admin/users/${param0}/disable */
export async function disableAdminUser(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.disableAdminUserParams,
  body: API.AdminRevisionRequest,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<API.AdminMutationReceipt>(
    `/api/admin/users/${param0}/disable`,
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

/** 此处后端没有提供注释 POST /api/admin/users/${param0}/enable */
export async function enableAdminUser(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.enableAdminUserParams,
  body: API.AdminRevisionRequest,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<API.AdminMutationReceipt>(
    `/api/admin/users/${param0}/enable`,
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

/** 此处后端没有提供注释 POST /api/admin/users/${param0}/password */
export async function resetAdminPassword(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.resetAdminPasswordParams,
  body: API.AdminPasswordRequest,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<API.AdminMutationReceipt>(
    `/api/admin/users/${param0}/password`,
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

/** 此处后端没有提供注释 POST /api/admin/users/${param0}/unlock */
export async function unlockAdminUser(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.unlockAdminUserParams,
  body: API.AdminRevisionRequest,
  options?: RequestOptions
) {
  const { id: param0, ...queryParams } = params;
  return request<API.AdminMutationReceipt>(
    `/api/admin/users/${param0}/unlock`,
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

/** Change a password and rotate this device's session POST /api/me/password */
export async function changePassword(
  body: API.PasswordChange,
  options?: RequestOptions
) {
  return request<API.SessionView>("/api/me/password", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** Read the current session, including a restricted account GET /api/session */
export async function getSession(options?: RequestOptions) {
  return request<API.SessionView>("/api/session", {
    method: "GET",
    ...(options || {}),
  });
}

/** Establish a real session POST /api/sessions */
export async function createSession(
  body: API.LoginRequest,
  options?: RequestOptions
) {
  return request<API.SessionView>("/api/sessions", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** Revoke the current device session DELETE /api/sessions/${param0} */
export async function deleteSession(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.deleteSessionParams,
  options?: RequestOptions
) {
  const { session_id: param0, ...queryParams } = params;
  return request<any>(`/api/sessions/${param0}`, {
    method: "DELETE",
    params: { ...queryParams },
    ...(options || {}),
  });
}

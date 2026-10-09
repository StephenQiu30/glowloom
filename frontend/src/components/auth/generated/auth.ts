// @ts-ignore
/* eslint-disable */
import { request } from "../request";

/** Check login availability before registration GET /api/accounts/availability */
export async function getLoginAvailability(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: AuthAPI.getLoginAvailabilityParams,
  options?: import("../request").RequestOptions,
) {
  return request<AuthAPI.AvailabilityView>("/api/accounts/availability", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** Register a creator and establish a session POST /api/accounts/register */
export async function registerAccount(
  body: AuthAPI.RegisterRequest,
  options?: import("../request").RequestOptions,
) {
  return request<AuthAPI.SessionView>("/api/accounts/register", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** Change a password and rotate this device's session POST /api/me/password */
export async function changePassword(
  body: AuthAPI.PasswordChange,
  options?: import("../request").RequestOptions,
) {
  return request<AuthAPI.SessionView>("/api/me/password", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** Read the current session, including a restricted account GET /api/session */
export async function getSession(
  options?: import("../request").RequestOptions,
) {
  return request<AuthAPI.SessionView>("/api/session", {
    method: "GET",
    ...(options || {}),
  });
}

/** Establish a real session POST /api/sessions */
export async function createSession(
  body: AuthAPI.LoginRequest,
  options?: import("../request").RequestOptions,
) {
  return request<AuthAPI.SessionView>("/api/sessions", {
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
  params: AuthAPI.deleteSessionParams,
  options?: import("../request").RequestOptions,
) {
  const { session_id: param0, ...queryParams } = params;
  return request<any>(`/api/sessions/${param0}`, {
    method: "DELETE",
    params: { ...queryParams },
    ...(options || {}),
  });
}

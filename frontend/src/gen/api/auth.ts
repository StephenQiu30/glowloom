// @ts-ignore
/* eslint-disable */
import { request } from "@/lib/request";

/** 密码登录 POST /api/auth/login */
export async function login(
  body: API.httpLoginRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  return request<API.httpSessionResponse>("/api/auth/login", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 登出 POST /api/auth/logout */
export async function logout(options?: import("@/lib/request").RequestOptions) {
  return request<any>("/api/auth/logout", {
    method: "POST",
    ...(options || {}),
  });
}

/** 当前账号 GET /api/auth/me */
export async function currentUser(
  options?: import("@/lib/request").RequestOptions,
) {
  return request<API.httpSessionResponse>("/api/auth/me", {
    method: "GET",
    ...(options || {}),
  });
}

/** 修改本人密码 POST /api/auth/password */
export async function changePassword(
  body: API.httpPasswordRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  return request<API.httpPasswordResponse>("/api/auth/password", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

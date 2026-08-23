// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/request";

/** 邮箱登录 POST /api/auth/login */
export async function authLogin(
  body: API.loginRequest,
  options?: RequestOptions
) {
  return request<API.AuthResponseEnvelope>("/api/auth/login", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 退出登录 POST /api/auth/logout */
export async function authLogout(options?: RequestOptions) {
  return request<any>("/api/auth/logout", {
    method: "POST",
    ...(options || {}),
  });
}

/** 获取当前身份 GET /api/auth/me */
export async function authMe(options?: RequestOptions) {
  return request<API.CurrentIdentityEnvelope>("/api/auth/me", {
    method: "GET",
    ...(options || {}),
  });
}

/** 无感刷新登录会话 POST /api/auth/refresh */
export async function authRefresh(options?: RequestOptions) {
  return request<API.AuthResponseEnvelope>("/api/auth/refresh", {
    method: "POST",
    ...(options || {}),
  });
}

/** 邮箱注册 POST /api/auth/register */
export async function authRegister(
  body: API.registerRequest,
  options?: RequestOptions
) {
  return request<API.AuthResponseEnvelope>("/api/auth/register", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

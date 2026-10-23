/* eslint-disable */
import { request } from "@/components/canvas/runtime/lib/request";
import type { AxiosRequestConfig } from "axios";

/** 登录并轮换 HttpOnly 会话 POST /account/login */
export async function loginAccount(
  body: CanvasContract.UserLoginRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseBrowserSessionVO>(
    "/account/login",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      data: body,
      ...(options || {}),
    },
  );
}

/** 退出当前会话 POST /account/logout */
export async function logoutAccount(options?: AxiosRequestConfig) {
  return request<CanvasContract.BaseResponseBoolean>("/account/logout", {
    method: "POST",
    ...(options || {}),
  });
}

/** 修改密码并退出全部会话 POST /account/password */
export async function changePassword(
  body: CanvasContract.UserUpdatePasswordRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseBoolean>("/account/password", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 按版本保存本人资料 POST /account/profile */
export async function saveProfile(
  body: CanvasContract.UserEditRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseBoolean>("/account/profile", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 注册账户 POST /account/register */
export async function registerAccount(
  body: CanvasContract.UserRegisterRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseLong>("/account/register", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 获取当前账户与防伪令牌 GET /account/session */
export async function browserSession(options?: AxiosRequestConfig) {
  return request<CanvasContract.BaseResponseBrowserSessionVO>(
    "/account/session",
    {
      method: "GET",
      ...(options || {}),
    },
  );
}

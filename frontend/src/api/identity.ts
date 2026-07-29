// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/request";

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

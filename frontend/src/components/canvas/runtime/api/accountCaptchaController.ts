/* eslint-disable */
import { request } from "@/components/canvas/runtime/lib/request";
import type { AxiosRequestConfig } from "axios";

/** 生成登录或注册拼图 POST /account/captcha/challenge */
export async function captchaChallenge(
  body: CanvasContract.CaptchaChallengeRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseCaptchaChallengeVO>(
    "/account/captcha/challenge",
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

/** 验证拼图并取得单次凭据 POST /account/captcha/verify */
export async function captchaVerify(
  body: CanvasContract.CaptchaVerifyRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseCaptchaTicketVO>(
    "/account/captcha/verify",
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

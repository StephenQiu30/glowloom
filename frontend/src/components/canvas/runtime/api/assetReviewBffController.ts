/* eslint-disable */
import { request } from "@/components/canvas/runtime/lib/request";
import type { AxiosRequestConfig } from "axios";

/** 核对原审核命令 GET /assets/review/command */
export async function assetReviewCommand(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CanvasContract.assetReviewCommandParams,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseAssetReviewVO>(
    "/assets/review/command",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    },
  );
}

/** 读取素材当前审核 GET /assets/review/current */
export async function assetReviewCurrent(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CanvasContract.assetReviewCurrentParams,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseAssetReviewVO>(
    "/assets/review/current",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    },
  );
}

/** 提交人工预审决定 POST /assets/review/decide */
export async function assetReviewDecide(
  body: CanvasContract.AssetReviewDecisionRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseAssetReviewVO>(
    "/assets/review/decide",
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

/** 查看审核申请 GET /assets/review/detail */
export async function assetReviewDetail(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CanvasContract.assetReviewDetailParams,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseAssetReviewVO>(
    "/assets/review/detail",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    },
  );
}

/** 分页读取审核待办 POST /assets/review/pending/list/page */
export async function assetReviewPending(
  body: CanvasContract.PageRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseAssetReviewPageVO>(
    "/assets/review/pending/list/page",
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

/** 核对原公共发布命令 GET /assets/review/public/command */
export async function assetReviewPublicCommand(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CanvasContract.assetReviewPublicCommandParams,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponsePublicAssetVO>(
    "/assets/review/public/command",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    },
  );
}

/** 查看公共素材固定版本 GET /assets/review/public/detail */
export async function assetReviewPublicDetail(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CanvasContract.assetReviewPublicDetailParams,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponsePublicAssetVO>(
    "/assets/review/public/detail",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    },
  );
}

/** 分页检索公共素材 POST /assets/review/public/list/page/vo */
export async function listPublicAssets(
  body: CanvasContract.AssetSearchQueryRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponsePublicAssetPageVO>(
    "/assets/review/public/list/page/vo",
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

/** 读取公共发布管理记录 GET /assets/review/public/managed */
export async function assetReviewManaged(options?: AxiosRequestConfig) {
  return request<CanvasContract.BaseResponseListPublicAssetVO>(
    "/assets/review/public/managed",
    {
      method: "GET",
      ...(options || {}),
    },
  );
}

/** 发布固定公共素材 POST /assets/review/public/publish */
export async function assetReviewPublish(
  body: CanvasContract.PublicAssetPublishRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponsePublicAssetVO>(
    "/assets/review/public/publish",
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

/** 下架或撤销公共许可 POST /assets/review/public/state */
export async function assetReviewState(
  body: CanvasContract.PublicAssetStateRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponsePublicAssetVO>(
    "/assets/review/public/state",
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

/** 按审核或公共许可查看固定原件 POST /assets/review/read */
export async function assetReviewRead(
  body: CanvasContract.AssetReadRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseFileSignedUrlVO>(
    "/assets/review/read",
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

/** 主动申请素材审核 POST /assets/review/submit */
export async function assetReviewSubmit(
  body: CanvasContract.AssetReviewSubmitRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseAssetReviewVO>(
    "/assets/review/submit",
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

/** 撤回审核或公共授权 POST /assets/review/withdraw */
export async function assetReviewWithdraw(
  body: CanvasContract.AssetReviewWithdrawRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseAssetReviewVO>(
    "/assets/review/withdraw",
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

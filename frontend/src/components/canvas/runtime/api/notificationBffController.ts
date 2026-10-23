/* eslint-disable */
import { request } from "@/components/canvas/runtime/lib/request";
import type { AxiosRequestConfig } from "axios";

/** 读取本人通知详情 GET /notification/get/vo */
export async function getNotificationVoById(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CanvasContract.getNotificationVOByIdParams,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseNotificationVO>(
    "/notification/get/vo",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    },
  );
}

/** 游标读取本人通知 POST /notification/my/list/page/vo */
export async function listMyNotifications(
  body: CanvasContract.NotificationQueryRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseNotificationPageVO>(
    "/notification/my/list/page/vo",
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

/** 读取本人通知摘要 GET /notification/my/summary */
export async function getMyNotificationSummary(options?: AxiosRequestConfig) {
  return request<CanvasContract.BaseResponseNotificationSummaryVO>(
    "/notification/my/summary",
    {
      method: "GET",
      ...(options || {}),
    },
  );
}

/** 标记本人单条通知已读 POST /notification/read */
export async function readNotification(
  body: CanvasContract.NotificationReadRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseNotificationSummaryVO>(
    "/notification/read",
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

/** 按快照水位标记已读 POST /notification/read/all */
export async function readAllNotifications(
  body: CanvasContract.NotificationReadAllRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseNotificationSummaryVO>(
    "/notification/read/all",
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

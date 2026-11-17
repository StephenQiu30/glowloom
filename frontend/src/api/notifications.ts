// @ts-ignore
/* eslint-disable */
import { request } from "@/lib/request";
import type { RequestOptions } from "@/lib/request";

/** 此处后端没有提供注释 GET /api/announcements/public */
export async function getPublicAnnouncements(options?: RequestOptions) {
  return request<API.NotificationDisplayView>("/api/announcements/public", {
    method: "GET",
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/notifications */
export async function listNotifications(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listNotificationsParams,
  options?: RequestOptions,
) {
  return request<API.NotificationPage>("/api/notifications", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/notifications/${param0} */
export async function getNotification(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getNotificationParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.InboxNotification>(`/api/notifications/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/notifications/${param0}/read */
export async function readNotification(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.readNotificationParams,
  body: API.ReadNotificationInput,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.NotificationSummary>(`/api/notifications/${param0}/read`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/notifications/display */
export async function getNotificationDisplay(options?: RequestOptions) {
  return request<API.NotificationDisplayView>("/api/notifications/display", {
    method: "GET",
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/notifications/display/${param0}/${param1} */
export async function validateNotificationDisplay(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.validateNotificationDisplayParams,
  options?: RequestOptions,
) {
  const { id: param0, revision: param1, ...queryParams } = params;
  return request<API.NotificationDisplayValidity>(
    `/api/notifications/display/${param0}/${param1}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 POST /api/notifications/display/ack */
export async function ackNotificationDisplay(
  body: API.NotificationDisplayAck,
  options?: RequestOptions,
) {
  return request<any>("/api/notifications/display/ack", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/notifications/read-all */
export async function readAllNotifications(
  body: API.ReadAllNotificationsInput,
  options?: RequestOptions,
) {
  return request<API.NotificationSummary>("/api/notifications/read-all", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/notifications/summary */
export async function getNotificationSummary(options?: RequestOptions) {
  return request<API.NotificationSummary>("/api/notifications/summary", {
    method: "GET",
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/platform/notifications */
export async function listPlatformNotifications(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listPlatformNotificationsParams,
  options?: RequestOptions,
) {
  return request<API.AnnouncementPage>("/api/platform/notifications", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /api/platform/notifications */
export async function createPlatformNotification(
  body: API.SaveAnnouncementInput,
  options?: RequestOptions,
) {
  return request<API.Announcement>("/api/platform/notifications", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/platform/notifications/${param0} */
export async function getPlatformNotification(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getPlatformNotificationParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.Announcement>(`/api/platform/notifications/${param0}`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 PUT /api/platform/notifications/${param0} */
export async function savePlatformNotification(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.savePlatformNotificationParams,
  body: API.SaveAnnouncementInput,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.Announcement>(`/api/platform/notifications/${param0}`, {
    method: "PUT",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/platform/notifications/${param0}/audit */
export async function getPlatformNotificationAudit(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getPlatformNotificationAuditParams,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.NotificationAuditPage>(
    `/api/platform/notifications/${param0}/audit`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 POST /api/platform/notifications/${param0}/delete */
export async function deletePlatformNotification(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.deletePlatformNotificationParams,
  body: API.PublishAnnouncementRequest,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.Announcement>(
    `/api/platform/notifications/${param0}/delete`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 PATCH /api/platform/notifications/${param0}/enabled */
export async function setPlatformNotificationEnabled(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.setPlatformNotificationEnabledParams,
  body: API.EnableAnnouncementRequest,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.Announcement>(
    `/api/platform/notifications/${param0}/enabled`,
    {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 POST /api/platform/notifications/${param0}/publish */
export async function publishPlatformNotification(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.publishPlatformNotificationParams,
  body: API.PublishAnnouncementRequest,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<API.Announcement>(
    `/api/platform/notifications/${param0}/publish`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

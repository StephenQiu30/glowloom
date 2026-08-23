import request, { type RequestOptions } from "@/lib/request";

/** List Schedules GET /api/v1/schedules */
export async function listSchedulesApiV1SchedulesGet(
  params: API.listSchedulesApiV1SchedulesGetParams,
  options?: RequestOptions,
) {
  return request<API.ApiResponsePaginatedSchedules_>(`/api/v1/schedules`, {
    method: "GET",
    params,
    ...(options ?? {}),
  });
}

/** Configure Schedule PUT /api/v1/schedules/{schedule_id}/configuration */
export async function configureScheduleApiV1SchedulesScheduleIdConfigurationPut(
  params: API.configureScheduleApiV1SchedulesScheduleIdConfigurationPutParams,
  body: API.ScheduleConfigurationRequest,
  options?: RequestOptions,
) {
  const { schedule_id: path0 } = params;
  return request<API.ApiResponseScheduleResponse_>(`/api/v1/schedules/${path0}/configuration`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Pause Schedule POST /api/v1/schedules/{schedule_id}/pause */
export async function pauseScheduleApiV1SchedulesScheduleIdPausePost(
  params: API.pauseScheduleApiV1SchedulesScheduleIdPausePostParams,
  body: API.ScheduleStateRequest,
  options?: RequestOptions,
) {
  const { schedule_id: path0 } = params;
  return request<API.ApiResponseScheduleResponse_>(`/api/v1/schedules/${path0}/pause`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Resume Schedule POST /api/v1/schedules/{schedule_id}/resume */
export async function resumeScheduleApiV1SchedulesScheduleIdResumePost(
  params: API.resumeScheduleApiV1SchedulesScheduleIdResumePostParams,
  body: API.ScheduleResumeRequest,
  options?: RequestOptions,
) {
  const { schedule_id: path0 } = params;
  return request<API.ApiResponseScheduleResponse_>(`/api/v1/schedules/${path0}/resume`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

/** Trigger Schedule POST /api/v1/schedules/{schedule_id}/trigger */
export async function triggerScheduleApiV1SchedulesScheduleIdTriggerPost(
  params: API.triggerScheduleApiV1SchedulesScheduleIdTriggerPostParams,
  body: API.ScheduleTriggerRequest,
  options?: RequestOptions,
) {
  const { schedule_id: path0 } = params;
  return request<API.ApiResponseScheduleFireResponse_>(`/api/v1/schedules/${path0}/trigger`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    data: body,
    ...(options ?? {}),
  });
}

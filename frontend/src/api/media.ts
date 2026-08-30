import request, { type RequestOptions } from "@/lib/request";

type Envelope<T> = { data: T };

export function initializeUploadApiV1MediaUploadsPost(
  body: API.UploadDeclaration,
  options?: RequestOptions,
) {
  return request<Envelope<API.UploadInitializationResponse>>(
    "/api/media/uploads",
    { method: "POST", data: body, ...(options ?? {}) },
  );
}

export function completeUploadApiV1MediaUploadsUploadSessionIdCompletePost(
  params: { upload_session_id: string },
  options?: RequestOptions,
) {
  return request<Envelope<API.UploadCompletionResponse>>(
    `/api/media/uploads/${params.upload_session_id}/complete`,
    { method: "POST", ...(options ?? {}) },
  );
}

export function getMediaApiV1MediaVersionIdGet(
  params: { version_id: string },
  options?: RequestOptions,
) {
  return request<Envelope<API.MediaVersionResponse>>(
    `/api/media/${params.version_id}`,
    { method: "GET", ...(options ?? {}) },
  );
}

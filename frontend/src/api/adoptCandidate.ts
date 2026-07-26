// @ts-ignore
/* eslint-disable */
import { request, type RequestOptions } from "@/lib/request";

/** Adopt Candidate POST /v1/adoptions */
export async function adoptCandidate(
  body: API.AdoptCandidateRequest,
  options?: RequestOptions
) {
  return request<API.AdoptionResponse>("/v1/adoptions", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

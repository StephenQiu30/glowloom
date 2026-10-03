import { randomUUID } from "node:crypto";
import { STATUS_CODES } from "node:http";

import { uploadMediaAsset } from "@/api/media";
import { backendApiOrigin } from "@/lib/backend-api-origin";

export const runtime = "nodejs";

const maxBodyBytes = 500 * 1024 * 1024 + 64 * 1024;
const uuidPattern =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const nilUUID = "00000000-0000-0000-0000-000000000000";

function problem(status: number, code: string, requestId: string): Response {
  const title = STATUS_CODES[status] ?? "Request Failed";
  return Response.json(
    {
      type: `https://lanverse.local/errors/${code}`,
      title,
      status,
      code,
      detail: title,
      request_id: requestId,
    },
    {
      status,
      headers: {
        "Content-Type": "application/problem+json",
        "Cache-Control": "private, no-store",
        "X-Request-Id": requestId,
      },
    },
  );
}

// This endpoint only adapts the upload transport. Go owns multipart parsing,
// Origin/key checks, workspace authorization, media validation and persistence.
export async function POST(
  request: Request,
  context: { params: Promise<{ projectId: string }> },
): Promise<Response> {
  const suppliedRequestId = request.headers.get("X-Request-Id");
  const requestId =
    suppliedRequestId && uuidPattern.test(suppliedRequestId)
      ? suppliedRequestId
      : randomUUID();
  const { projectId } = await context.params;
  if (!uuidPattern.test(projectId) || projectId === nilUUID) {
    return problem(422, "invalid_request", requestId);
  }

  const contentLength = request.headers.get("Content-Length");
  if (contentLength !== null) {
    const length = Number(contentLength);
    if (!/^\d+$/.test(contentLength) || !Number.isSafeInteger(length)) {
      return problem(422, "invalid_request", requestId);
    }
    if (length > maxBodyBytes)
      return problem(413, "request_too_large", requestId);
  }

  let origin: string;
  try {
    origin = backendApiOrigin(process.env.LV_API_BASE_URL);
  } catch {
    return problem(503, "dependency_unavailable", requestId);
  }

  const headers = new Headers();
  // An allowlist excludes authorization, cookies, Host, proxy identity and hop headers.
  // Missing Origin/key stay missing so Go cannot mistake this adapter for the caller.
  for (const name of [
    "Content-Type",
    "Content-Length",
    "Origin",
    "Idempotency-Key",
  ]) {
    const value = request.headers.get(name);
    if (value !== null) headers.set(name, value);
  }
  headers.set("X-Request-Id", requestId);

  const deadline = AbortSignal.timeout(300_000);
  const signal = AbortSignal.any([request.signal, deadline]);
  let bytes = 0;
  let tooLarge = false;
  const countedBody = request.body?.pipeThrough(
    new TransformStream<Uint8Array, Uint8Array>({
      transform(chunk, controller) {
        bytes += chunk.byteLength;
        if (bytes > maxBodyBytes) {
          tooLarge = true;
          throw new RangeError("Upload body exceeds its transport limit");
        }
        controller.enqueue(chunk);
      },
    }),
    { signal },
  );

  try {
    signal.throwIfAborted();
    let status = 502;
    const responseHeaders = new Headers({
      "Cache-Control": "private, no-store",
    });
    const body: unknown = await uploadMediaAsset(
      { pid: projectId },
      // The original multipart stream replaces the generator's FormData entirely.
      // Go reads the caller's confirmation field from that original stream.
      { local_review_confirmed: false },
      undefined,
      {
        baseURL: origin,
        adapter: "fetch",
        data: countedBody ?? null,
        headers: {
          accept: false,
          "user-agent": false,
          "content-type": false,
          ...Object.fromEntries(headers),
        },
        signal,
        timeout: 0,
        responseType: "stream",
        validateStatus: () => true,
        maxRedirects: 0,
        fetchOptions: {
          cache: "no-store",
          redirect: "manual",
        },
        onResponse(response) {
          status = response.status;
          for (const name of ["Content-Type", "X-Request-Id"]) {
            const value = response.headers[name.toLowerCase()];
            if (typeof value === "string") responseHeaders.set(name, value);
          }
        },
      },
    );
    if (body !== null && !(body instanceof ReadableStream))
      return problem(502, "dependency_unavailable", requestId);
    return new Response(body, {
      status,
      headers: responseHeaders,
    });
  } catch {
    if (tooLarge) return problem(413, "request_too_large", requestId);
    if (request.signal.aborted)
      return problem(408, "dependency_unavailable", requestId);
    if (deadline.aborted)
      return problem(504, "dependency_unavailable", requestId);
    return problem(502, "dependency_unavailable", requestId);
  }
}

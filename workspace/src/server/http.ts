import { authorize, AccessError, type Capability } from "./auth/session";
import { getConfig } from "./config";
export { cookieName } from "./auth/session";
import { cookieName } from "./auth/session";
export function tokenFrom(request: Request) {
  return (
    request.headers
      .get("cookie")
      ?.split(";")
      .map((s) => s.trim())
      .find((s) => s.startsWith(cookieName + "="))
      ?.slice(cookieName.length + 1) ?? ""
  );
}
export function requireAccess(
  request: Request,
  capability: Capability,
  write = false,
) {
  const c = getConfig();
  return authorize(
    {
      token: tokenFrom(request),
      origin: request.headers.get("origin") ?? "",
      expectedOrigin: c.origin,
      csrf: request.headers.get("x-workspace-csrf") ?? "",
    },
    c.secret,
    capability,
    Date.now(),
    write,
  );
}
export async function readBody(
  request: Request,
): Promise<Record<string, unknown>> {
  if (!request.headers.get("content-type")?.startsWith("application/json"))
    throw new AccessError("json_required", 415);
  const reader = request.body?.getReader();
  if (!reader) throw new AccessError("body_required", 400);
  let count = 0;
  let text = "";
  const decoder = new TextDecoder();
  while (true) {
    const { value, done } = await reader.read();
    if (done) break;
    count += value.byteLength;
    if (count > 2_500_000) {
      await reader.cancel();
      throw new AccessError("body_too_large", 413);
    }
    text += decoder.decode(value, { stream: true });
  }
  text += decoder.decode();
  let body: unknown;
  try {
    body = JSON.parse(text);
  } catch {
    throw new AccessError("invalid_json", 400);
  }
  if (!body || Array.isArray(body) || typeof body !== "object")
    throw new AccessError("object_required", 400);
  return body as Record<string, unknown>;
}
export function stringField(
  body: Record<string, unknown>,
  field: string,
  max = 160,
) {
  const value = body[field];
  if (typeof value !== "string" || !value.trim() || value.length > max)
    throw new AccessError(`invalid_${field}`, 400);
  return value;
}
export function errorResponse(error: unknown) {
  const code = error instanceof Error ? error.message : "internal_error";
  const known = [
    "version_conflict",
    "idempotency_conflict",
    "receipt_conflict",
    "Input or authorization stale",
    "Stage evidence required",
    "Matching passed evidence required",
    "Acceptance required",
  ];
  if (error instanceof AccessError)
    return Response.json({ error: error.code }, { status: error.status });
  if (known.includes(code))
    return Response.json({ error: code }, { status: 409 });
  if (code.endsWith("_not_found"))
    return Response.json({ error: code }, { status: 404 });
  console.error(
    JSON.stringify({
      event: "workspace_request_failed",
      code:
        error && typeof error === "object" && "code" in error
          ? String(error.code)
          : "internal",
    }),
  );
  return Response.json({ error: "operation_failed" }, { status: 500 });
}

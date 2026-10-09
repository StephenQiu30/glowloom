// The browser cannot choose an upstream host or use this slice as a general proxy.
const resources: Record<string, readonly string[]> = {
  "accounts/register": ["POST"],
  sessions: ["POST"],
  session: ["GET"],
  "me/password": ["POST"],
  "accounts/availability": ["GET"],
};
const noStore = { "Cache-Control": "no-store" };
async function proxy(
  request: Request,
  context: { params: Promise<{ path: string[] }> },
) {
  const { path } = await context.params;
  const resource = path.join("/");
  const methods =
    resources[resource] ??
    (/^sessions\/[0-9a-f-]{36}$/i.test(resource) ? ["DELETE"] : undefined);
  if (!methods)
    return Response.json(
      { code: "not_found" },
      { status: 404, headers: noStore },
    );
  if (!methods.includes(request.method))
    return Response.json(
      { code: "method_not_allowed" },
      { status: 405, headers: { ...noStore, Allow: methods.join(", ") } },
    );
  try {
    const upstream = new URL(
      `/api/${resource}`,
      process.env.LV_BACKEND_ORIGIN ?? "http://127.0.0.1:8080",
    );
    const incoming = new URL(request.url);
    if (resource === "accounts/availability")
      upstream.searchParams.set(
        "login_name",
        incoming.searchParams.get("login_name") ?? "",
      );
    const headers = new Headers();
    for (const key of [
      "Cookie",
      "Origin",
      "Idempotency-Key",
      "Content-Type",
      "X-Request-Id",
    ]) {
      const value = request.headers.get(key);
      if (value) headers.set(key, value);
    }
    // Read incrementally so a hostile body cannot allocate unbounded memory.
    const reader = request.body?.getReader();
    const chunks: Uint8Array[] = [];
    let size = 0;
    if (reader) {
      try {
        while (true) {
          const next = await reader.read();
          if (next.done) break;
          size += next.value.byteLength;
          if (size > 1048576) {
            await reader.cancel();
            return Response.json(
              { code: "request_too_large" },
              { status: 413, headers: noStore },
            );
          }
          chunks.push(next.value);
        }
      } finally {
        reader.releaseLock();
      }
    }
    const body = size ? Buffer.concat(chunks) : undefined;
    const response = await fetch(upstream, {
      method: request.method,
      headers,
      body,
      cache: "no-store",
      redirect: "manual",
      signal: AbortSignal.timeout(10000),
    });
    const outgoing = new Headers(noStore);
    for (const key of ["Content-Type", "X-Request-Id"]) {
      const value = response.headers.get(key);
      if (value) outgoing.set(key, value);
    }
    for (const cookie of response.headers.getSetCookie())
      outgoing.append("Set-Cookie", cookie);
    return new Response(response.body, {
      status: response.status,
      headers: outgoing,
    });
  } catch {
    return Response.json(
      { code: "dependency_unavailable" },
      { status: 503, headers: noStore },
    );
  }
}
export const GET = proxy;
export const POST = proxy;
export const DELETE = proxy;

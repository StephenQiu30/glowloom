// The browser cannot choose an upstream host or use this slice as a general proxy.
const resources: Record<string, readonly string[]> = {
  "accounts/register": ["POST"],
  sessions: ["POST"],
  session: ["GET"],
  "me/password": ["POST"],
  "accounts/availability": ["GET"],
  projects: ["GET", "POST"],
  "media/library": ["GET"],
  "media/library/commands": ["POST"],
  "media/library/uploads": ["POST"],
};
const noStore = { "Cache-Control": "no-store" };
async function proxy(
  request: Request,
  context: { params: Promise<{ path: string[] }> },
) {
  const { path } = await context.params;
  const resource = path.join("/");
  const uuid = "[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}";
  const matches = (pattern: string) =>
    new RegExp(`^${pattern}$`, "i").test(resource);
  const flowMethods =
    resource === "admin/users"
      ? ["GET", "POST"]
      : matches(`admin/users/${uuid}`)
        ? ["PUT"]
        : matches(`admin/users/${uuid}/(disable|enable|password|unlock)`)
          ? ["POST"]
          : resource === "models"
            ? ["GET"]
            : matches(`projects/${uuid}/(operations|media)`)
              ? ["GET"]
              : matches(`projects/${uuid}/(free-operations|media/uploads)`)
                ? ["POST"]
                : matches(`projects/${uuid}/media/${uuid}/preview`)
                  ? ["GET"]
                  : matches(`operations/${uuid}`)
                    ? ["GET"]
                    : matches(`operations/${uuid}/(confirm|cancel)`)
                      ? ["POST"]
                      : matches(
                            `projects/${uuid}/bible/(character|location|prop)`,
                          )
                        ? ["GET", "POST"]
                        : matches(
                              `projects/${uuid}/bible/(character|location|prop)/${uuid}`,
                            )
                          ? ["GET", "PUT", "DELETE"]
                          : matches(
                                `projects/${uuid}/bible/(character|location|prop)/${uuid}/(restore|confirm|looks)`,
                              )
                            ? ["POST"]
                            : matches(
                                  `projects/${uuid}/bible/character/${uuid}/looks/${uuid}/references`,
                                )
                              ? ["PUT"]
                              : matches(
                                    `projects/${uuid}/bible/(character|location|prop)/${uuid}/versions`,
                                  )
                                ? ["GET"]
                                : undefined;
  const methods =
    flowMethods ??
    resources[resource] ??
    (matches(`projects/${uuid}/canvases`) ? ["GET", "POST"] : undefined) ??
    (/^media\/library\/items\/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}(\/(preview|download))?$/i.test(
      resource,
    )
      ? ["GET"]
      : undefined) ??
    (/^sessions\/[0-9a-f-]{36}$/i.test(resource)
      ? ["DELETE"]
      : /^canvases\/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\/commands$/i.test(
            resource,
          )
        ? ["POST"]
        : /^(projects|canvases)\/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(
              resource,
            )
          ? ["GET"]
          : undefined);
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
  let uploadTooLarge = false;
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
    if (resource === "projects")
      for (const key of ["q", "limit", "cursor"])
        if (incoming.searchParams.has(key))
          upstream.searchParams.set(key, incoming.searchParams.get(key)!);
    if (flowMethods && request.method === "GET")
      for (const key of [
        "project_id",
        "kind",
        "capability",
        "mode",
        "limit",
        "cursor",
        "status",
        "canvas_id",
        "node_id",
        "origin",
        "row_id",
        "model_key",
        "deleted",
      ])
        if (incoming.searchParams.has(key))
          upstream.searchParams.set(key, incoming.searchParams.get(key)!);
    if (resource.startsWith("media/library") && request.method === "GET") {
      const allowed =
        resource === "media/library"
          ? [
              "scope",
              "page",
              "page_size",
              "kind",
              "category",
              "folder_id",
              "root_only",
              "favorite_only",
              "recent_only",
              "catalog_state",
              "search",
              "order",
            ]
          : ["scope"];
      for (const key of allowed)
        if (incoming.searchParams.has(key))
          upstream.searchParams.set(key, incoming.searchParams.get(key)!);
      // This frontend currently exposes personal assets only. Project reads remain separately scoped.
      upstream.searchParams.set("scope", "personal");
    }
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
    if (
      resource === "media/library/uploads" ||
      matches(`projects/${uuid}/media/uploads`)
    ) {
      let bytes = 0;
      const body = request.body?.pipeThrough(
        new TransformStream({
          transform(chunk, controller) {
            bytes += chunk.byteLength;
            if (bytes > 501 * 1024 * 1024) {
              uploadTooLarge = true;
              controller.error(new Error("upload_too_large"));
              return;
            }
            controller.enqueue(chunk);
          },
        }),
      );
      const response = await fetch(upstream, {
        method: "POST",
        headers,
        body,
        duplex: "half",
        cache: "no-store",
        redirect: "manual",
        signal: AbortSignal.any([request.signal, AbortSignal.timeout(120000)]),
      } as RequestInit & { duplex: "half" });
      return new Response(response.body, {
        status: response.status,
        headers: {
          ...noStore,
          "Content-Type":
            response.headers.get("Content-Type") ?? "application/json",
        },
      });
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
      signal: AbortSignal.any([request.signal, AbortSignal.timeout(10000)]),
    });
    const outgoing = new Headers(noStore);
    for (const key of [
      "Content-Type",
      "X-Request-Id",
      "Content-Disposition",
      "X-Content-Type-Options",
    ]) {
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
    if (uploadTooLarge)
      return Response.json(
        { code: "request_too_large" },
        { status: 413, headers: noStore },
      );
    return Response.json(
      { code: "dependency_unavailable" },
      { status: 503, headers: noStore },
    );
  }
}
export const GET = proxy;
export const POST = proxy;
export const DELETE = proxy;

export const PUT = proxy;

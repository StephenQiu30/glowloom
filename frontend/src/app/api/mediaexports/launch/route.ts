const headers = {
  "Cache-Control": "no-store",
  "Referrer-Policy": "no-referrer",
  "X-Content-Type-Options": "nosniff",
};

/** A narrow native-app capability relay. Browser identity is never forwarded. */
export async function GET(request: Request) {
  if (request.method !== "GET")
    return Response.json(
      { code: "method_not_allowed" },
      { status: 405, headers: { ...headers, Allow: "GET" } },
    );
  const incoming = new URL(request.url);
  const token = incoming.searchParams.get("token");
  if (
    [...incoming.searchParams.keys()].length !== 1 ||
    !token ||
    !/^[a-f0-9]{64}$/.test(token)
  )
    return Response.json({ code: "invalid_request" }, { status: 422, headers });
  try {
    const upstream = new URL(
      "/api/media-export-launch",
      process.env.GLOWLOOM_BACKEND_ORIGIN ??
        process.env.LV_BACKEND_ORIGIN ??
        "http://127.0.0.1:8080",
    );
    upstream.searchParams.set("token", token);
    const response = await fetch(upstream, {
      headers: { Accept: "application/json" },
      credentials: "omit",
      cache: "no-store",
      redirect: "manual",
      signal: AbortSignal.any([request.signal, AbortSignal.timeout(10000)]),
    });
    if (!response.ok) {
      await response.body?.cancel();
      const status = [404, 410, 429].includes(response.status)
        ? response.status
        : 502;
      return Response.json(
        {
          code:
            status === 404 || status === 410
              ? "not_found"
              : "dependency_unavailable",
        },
        { status, headers },
      );
    }
    if (
      response.headers
        .get("Content-Type")
        ?.split(";")[0]
        .trim()
        .toLowerCase() !== "application/json"
    ) {
      await response.body?.cancel();
      throw new Error("invalid launch receipt");
    }
    const reader = response.body?.getReader();
    if (!reader) throw new Error("empty launch receipt");
    const chunks: Uint8Array[] = [];
    let size = 0;
    try {
      while (true) {
        const chunk = await reader.read();
        if (chunk.done) break;
        size += chunk.value.byteLength;
        if (size > 16384) {
          await reader.cancel();
          throw new Error("large launch receipt");
        }
        chunks.push(chunk.value);
      }
    } finally {
      reader.releaseLock();
    }
    const body = Buffer.concat(chunks);
    const value: unknown = JSON.parse(body.toString("utf8"));
    if (!value || typeof value !== "object" || Array.isArray(value))
      throw new Error("invalid launch receipt");
    return new Response(body, {
      headers: { ...headers, "Content-Type": "application/json" },
    });
  } catch {
    return Response.json(
      { code: "dependency_unavailable" },
      { status: 502, headers },
    );
  }
}

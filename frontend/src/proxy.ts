import { NextResponse, type NextRequest } from "next/server";
import { canonicalNavigation } from "./lib/canonical-origin";

// These are public, build-owned files. Next's internal image request has no Host
// header; application routes and the optimizer endpoint still validate Host.
export const config = {
  matcher: ["/((?!_next/static/|design/reference/).*)"],
};

export function proxy(request: NextRequest) {
  const canonicalOrigin =
    process.env.GLOWLOOM_PUBLIC_ORIGIN ??
    process.env.LV_PUBLIC_ORIGIN ??
    (process.env.NODE_ENV === "development"
      ? "http://127.0.0.1:3200"
      : undefined);
  const problem = (
    code: string,
    status: number,
    meta?: { canonical_origin: string },
  ) =>
    NextResponse.json(
      { code, ...(meta ? { meta } : {}) },
      { status, headers: { "Cache-Control": "no-store" } },
    );
  const incoming = new URL(request.url);
  if (
    request.headers.get("host") === "127.0.0.1:3000" &&
    incoming.pathname === "/healthz" &&
    !incoming.search &&
    (request.method === "GET" || request.method === "HEAD")
  )
    return NextResponse.next();
  if (!canonicalOrigin) return problem("site_origin_unavailable", 503);
  try {
    const canonical = new URL(canonicalOrigin);
    const host = request.headers.get("host");
    if (
      !host ||
      (host !== canonical.host &&
        !(
          canonicalOrigin === "http://127.0.0.1:3200" &&
          host === "localhost:3200"
        ))
    )
      return problem("invalid_host", 400);
    // Next's URL can carry the internal Docker host/port. Trust only validated Host.
    const actual = new URL(request.url);
    actual.protocol = canonical.protocol;
    actual.host = host;
    const navigation = canonicalNavigation(
      actual,
      request.method,
      canonicalOrigin,
    );
    if (navigation)
      return NextResponse.redirect(navigation, {
        status: 307,
        headers: { "Cache-Control": "no-store" },
      });
    if (actual.origin !== canonical.origin)
      return problem("origin_mismatch", 403, {
        canonical_origin: canonical.origin,
      });
    return NextResponse.next();
  } catch {
    return problem("invalid_request", 400);
  }
}

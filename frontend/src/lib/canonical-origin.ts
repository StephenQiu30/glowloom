/** Validate navigation without reading server configuration or browser state. */
export function canonicalNavigation(
  requestURL: URL,
  method: string,
  canonicalOrigin: string,
): URL | null {
  const canonical = new URL(canonicalOrigin);
  if (
    !/^https?:$/.test(canonical.protocol) ||
    canonical.origin !== canonicalOrigin ||
    canonical.username ||
    canonical.password
  )
    throw new Error("invalid_canonical_origin");
  // Only the documented local alias is trusted, never arbitrary loopback hosts.
  const alias =
    canonicalOrigin === "http://127.0.0.1:3200" &&
    requestURL.origin === "http://localhost:3200";
  if (
    requestURL.username ||
    requestURL.password ||
    (requestURL.origin !== canonical.origin && !alias)
  )
    throw new Error("invalid_host");
  for (const target of requestURL.searchParams.getAll("return_to")) {
    let decoded = target;
    for (let i = 0; i < 8; i++) {
      if (
        !decoded.startsWith("/") ||
        decoded.startsWith("//") ||
        /[\\\x00-\x20\x7f]/.test(decoded)
      )
        throw new Error("invalid_return_to");
      const next = decodeURIComponent(decoded);
      if (next === decoded) break;
      if (i === 7) throw new Error("invalid_return_to");
      decoded = next;
    }
    if (new URL(decoded, canonical).origin !== canonical.origin)
      throw new Error("invalid_return_to");
  }
  if (!alias || (method !== "GET" && method !== "HEAD")) return null;
  // Assignment preserves path and safe query without URL-relative host resolution.
  canonical.pathname = requestURL.pathname;
  canonical.search = requestURL.search;
  return canonical;
}

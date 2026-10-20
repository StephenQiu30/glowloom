import type { RequestOptions } from "./request";

type LocalTransport = {
  journal?: object;
  matches: (path: string) => boolean;
  request: (url: string, options: RequestOptions) => Promise<unknown>;
};
let active: LocalTransport | null = null;
export function developmentJournal(): object | null {
  return process.env.NODE_ENV !== "production" &&
    active &&
    typeof window !== "undefined" &&
    active.matches(window.location.pathname)
    ? (active.journal ?? null)
    : null;
}
export function developmentMediaURL(url: string): boolean {
  return (
    process.env.NODE_ENV !== "production" &&
    !!active &&
    typeof window !== "undefined" &&
    active.matches(window.location.pathname) &&
    url.startsWith(`blob:${window.location.origin}/`)
  );
}

/** Explicit development preview seam; generated API functions remain unchanged. */
export function installDevelopmentTransport(transport: LocalTransport) {
  if (process.env.NODE_ENV === "production")
    throw new Error("Development transport is disabled in production");
  if (active) throw new Error("A development transport is already installed");
  active = transport;
  return () => {
    if (active === transport) active = null;
  };
}
export function developmentResponse(
  url: string,
  options: RequestOptions,
): Promise<unknown> | null {
  if (
    process.env.NODE_ENV === "production" ||
    !active ||
    typeof window === "undefined" ||
    !active.matches(window.location.pathname)
  )
    return null;
  if (options.signal?.aborted)
    return Promise.reject(
      new DOMException("Preview request cancelled", "AbortError"),
    );
  return active.request(url, options).then((value) => {
    if (options.signal?.aborted)
      throw new DOMException("Preview request cancelled", "AbortError");
    return value;
  });
}

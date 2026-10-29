/** Keep authentication navigation on this site and outside authentication loops. */
function localDestination(value: string | null): string {
  if (
    !value?.startsWith("/") ||
    value.startsWith("//") ||
    /[\\\u0000-\u0020]/.test(value)
  )
    return "/";
  try {
    const target = new URL(value, "https://local.invalid");
    if (
      target.origin !== "https://local.invalid" ||
      /^\/(login|register|reset-password|recover)(\/|$)/.test(target.pathname)
    )
      return "/";
    return target.pathname + target.search + target.hash;
  } catch {
    return "/";
  }
}

export function loginLocation(destination: string): string {
  const target = localDestination(destination);
  return target === "/"
    ? "/login"
    : `/login?returnTo=${encodeURIComponent(target)}`;
}

export function authenticationReturn(search: string): string {
  return localDestination(new URLSearchParams(search).get("returnTo"));
}

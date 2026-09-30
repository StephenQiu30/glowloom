/** Redirect only to an application workspace. */
export function loginDestination(value: string | null | undefined) {
  if (
    !value ||
    !value.startsWith("/") ||
    value.startsWith("//") ||
    value.includes("\\")
  )
    return "/canvas";
  const target = new URL(value, "https://lanverse.invalid");
  if (
    target.origin !== "https://lanverse.invalid" ||
    !/^\/(canvas|projects)(\/|$)/.test(target.pathname)
  )
    return "/canvas";
  return target.pathname + target.search + target.hash;
}

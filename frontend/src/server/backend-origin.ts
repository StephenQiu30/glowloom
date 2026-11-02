type BackendEnvironment = Readonly<Record<string, string | undefined>>;

export function getBackendOrigin(
  environment: BackendEnvironment = process.env,
): string {
  const value = environment.GLOWLOOM_BACKEND_ORIGIN;
  if (!value?.trim()) {
    throw new Error("GLOWLOOM_BACKEND_ORIGIN is required");
  }

  let origin: URL;
  try {
    origin = new URL(value.trim());
  } catch {
    throw new Error("GLOWLOOM_BACKEND_ORIGIN must be an HTTP(S) origin");
  }

  if (
    (origin.protocol !== "http:" && origin.protocol !== "https:") ||
    origin.username ||
    origin.password ||
    origin.pathname !== "/" ||
    origin.search ||
    origin.hash
  ) {
    throw new Error("GLOWLOOM_BACKEND_ORIGIN must be an HTTP(S) origin");
  }

  return origin.origin;
}

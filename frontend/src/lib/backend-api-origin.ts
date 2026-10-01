// Shared only by the Next configuration and server-side upload route.
export function backendApiOrigin(value: string | undefined): string {
  const message = "LV_API_BASE_URL 必须是无凭据的 HTTP(S) 服务根地址。";
  let root: URL;
  try {
    root = new URL(value ?? "http://127.0.0.1:8080");
  } catch {
    // URL parsing errors can echo credentials from configuration.
    throw new Error(message);
  }
  if (
    !["http:", "https:"].includes(root.protocol) ||
    root.username ||
    root.password ||
    root.search ||
    root.hash ||
    root.pathname !== "/"
  ) {
    throw new Error(message);
  }
  return root.origin;
}

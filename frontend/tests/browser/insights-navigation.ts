const pages: Record<string, string> = {
  diagnostics: "/health",
  audit: "/audit",
  creation: "/analytics",
  business: "/analytics",
};
export function usePathname() {
  return (
    pages[new URLSearchParams(location.search).get("page") ?? "diagnostics"] ??
    "/health"
  );
}
export function useRouter() {
  return {
    push: (url: string) => {
      const next = new URL(location.href);
      const path = new URL(url, location.origin).pathname;
      next.searchParams.set(
        "page",
        path === "/audit"
          ? "audit"
          : path === "/analytics"
            ? "creation"
            : "diagnostics",
      );
      location.href = next.href;
    },
    replace: () => {},
    back: () => history.back(),
    refresh: () => location.reload(),
  };
}
export function useSearchParams() {
  return new URLSearchParams(location.search);
}

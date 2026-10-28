/** Test-only navigation keeps the isolated fixture origin and never opens product routes. */
export function useRouter() {
  return {
    push: (url: string) => {
      const view = new URL(url, location.origin).searchParams.get("view");
      const next = new URL(location.href);
      if (view) next.searchParams.set("view", view);
      else next.searchParams.delete("view");
      history.pushState({}, "", next);
    },
    replace: () => {},
    back: () => history.back(),
    refresh: () => location.reload(),
  };
}
export function useSearchParams() {
  return new URLSearchParams(location.search);
}
export function usePathname() {
  return "/assets";
}

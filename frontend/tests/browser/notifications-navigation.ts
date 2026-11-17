/** Dedicated fixture routes; navigation never enters product URLs. */
export function fixtureURL(href: string) {
  const target = new URL(href, location.origin);
  const next = new URL(location.href);
  next.searchParams.set("route", target.pathname);
  next.searchParams.delete("notification");
  target.searchParams.forEach((value, key) =>
    next.searchParams.set(key, value),
  );
  return next.href;
}
export function usePathname() {
  return new URLSearchParams(location.search).get("route") ?? "/";
}
const router = {
  push: (href: string) => location.assign(fixtureURL(href)),
  replace: (href: string) => location.replace(fixtureURL(href)),
  back: () => history.back(),
  refresh: () => location.reload(),
};
export function useRouter() {
  return router;
}
export function useSearchParams() {
  return new URLSearchParams(location.search);
}

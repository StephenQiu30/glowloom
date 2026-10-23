// Navigation for the standalone canvas fixture; application routes use Next.js.
export function useRouter() {
  return {
    push: (url: string) => window.location.assign(url),
    replace: (url: string) => window.location.replace(url),
    back: () => window.history.back(),
    refresh: () => window.location.reload(),
  };
}
export function useSearchParams() {
  return new URLSearchParams(window.location.search);
}
export function usePathname() {
  return window.location.pathname;
}

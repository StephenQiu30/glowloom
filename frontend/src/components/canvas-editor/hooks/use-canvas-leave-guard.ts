"use client";
import { useEffect, useRef } from "react";

/** Keep the mounted session until its graph and original command are resolved. */
export function useCanvasLeaveGuard(pending: boolean) {
  const blocked = useRef(pending);
  useEffect(() => {
    blocked.current = pending;
  }, [pending]);
  useEffect(() => {
    if (!pending) return;
    const url = window.location.href;
    const entry = window.history.state;
    const close = (event: BeforeUnloadEvent) => {
      if (!blocked.current) return;
      event.preventDefault();
      event.returnValue = "";
    };
    const leave = (event: MouseEvent) => {
      const anchor = (event.target as Element).closest("a[href]");
      if (!blocked.current || !anchor || anchor.hasAttribute("download"))
        return;
      event.preventDefault();
      event.stopPropagation();
    };
    // Cancel traversal before the URL or Next's route state changes when supported.
    const navigation = (window as Window & { navigation?: EventTarget })
      .navigation;
    const navigate = (event: Event) => {
      if (
        blocked.current &&
        (event as Event & { navigationType?: string }).navigationType ===
          "traverse" &&
        event.cancelable
      )
        event.preventDefault();
    };
    // Older browsers: capture precedes Next's bubbling popstate handler. Restore
    // this entry without routing away; forward history may be replaced here.
    const traverse = (event: PopStateEvent) => {
      if (!blocked.current) return;
      event.stopImmediatePropagation();
      window.history.pushState(entry, "", url);
    };
    window.addEventListener("beforeunload", close);
    document.addEventListener("click", leave, true);
    navigation?.addEventListener("navigate", navigate);
    window.addEventListener("popstate", traverse, true);
    return () => {
      window.removeEventListener("beforeunload", close);
      document.removeEventListener("click", leave, true);
      navigation?.removeEventListener("navigate", navigate);
      window.removeEventListener("popstate", traverse, true);
    };
  }, [pending]);
  return () => {
    blocked.current = false;
  };
}

"use client";

import {
  useEffect,
  useMemo,
  useSyncExternalStore,
  type RefObject,
} from "react";

// Every visible node shares this one document subscription.
const pageSubscribers = new Set<() => void>();
function notifyPage() {
  pageSubscribers.forEach((notify) => notify());
}
function subscribePage(notify: () => void) {
  if (pageSubscribers.size === 0)
    document.addEventListener("visibilitychange", notifyPage);
  pageSubscribers.add(notify);
  return () => {
    pageSubscribers.delete(notify);
    if (pageSubscribers.size === 0)
      document.removeEventListener("visibilitychange", notifyPage);
  };
}
function pageVisible() {
  return !document.hidden;
}
function serverHidden() {
  return false;
}

function nodeVisibility(root: RefObject<HTMLElement | null>) {
  let visible = false;
  let viewport: Element | null = null;
  return {
    snapshot: () =>
      visible &&
      viewport?.getAttribute("data-canvas-viewport-interacting") !== "true" &&
      viewport?.getAttribute("data-canvas-node-dragging") !== "true",
    subscribe: (notify: () => void) => {
      const element = root.current;
      if (!element) return () => {};
      viewport = element.closest("[data-canvas-viewport]");
      const intersection = new IntersectionObserver((entries) => {
        visible = entries.some((entry) => entry.isIntersecting);
        notify();
      });
      intersection.observe(element);
      const mutation = new MutationObserver(notify);
      if (viewport)
        mutation.observe(viewport, {
          attributes: true,
          attributeFilter: [
            "data-canvas-viewport-interacting",
            "data-canvas-node-dragging",
          ],
        });
      return () => {
        intersection.disconnect();
        mutation.disconnect();
        visible = false;
      };
    },
  };
}

// DOM interaction flags update independently of React's document snapshot.
export function useMediaLifecycle(
  root: RefObject<HTMLElement | null>,
  active: boolean,
  inMotion: boolean,
) {
  const visibility = useMemo(() => nodeVisibility(root), [root]);
  const visible = useSyncExternalStore(
    visibility.subscribe,
    visibility.snapshot,
    serverHidden,
  );
  const page = useSyncExternalStore(subscribePage, pageVisible, serverHidden);
  return active && !inMotion && visible && page;
}

export function releaseMediaSource(
  element: HTMLMediaElement | HTMLImageElement,
) {
  if (element instanceof HTMLMediaElement) element.pause();
  element.removeAttribute("src");
  if (element instanceof HTMLMediaElement) element.load();
}

// Capture the current DOM node before React clears its ref during unmount.
export function useReleaseMediaSource(
  ref: RefObject<HTMLMediaElement | HTMLImageElement | null>,
  url: string,
) {
  useEffect(() => {
    const element = ref.current;
    // StrictMode replays setup after cleanup without reapplying JSX attributes.
    // Restore CORS first, then the authorized source on every owned setup.
    if (element) {
      element.crossOrigin = "anonymous";
      element.setAttribute("src", url);
    }
    return () => {
      if (element) releaseMediaSource(element);
    };
  }, [ref, url]);
}

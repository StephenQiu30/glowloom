'use client';
import { useSyncExternalStore } from 'react';
const query = '(max-width: 767px)';
function subscribe(listener: () => void) {
  const media = window.matchMedia(query);
  media.addEventListener('change', listener);
  return () => media.removeEventListener('change', listener);
}
function getSnapshot() {
  return window.matchMedia(query).matches;
}
function getServerSnapshot() {
  return false;
}
/** Uses one hydration-safe external subscription instead of an effect-driven state update. */
export function useIsMobile() {
  return useSyncExternalStore(subscribe, getSnapshot, getServerSnapshot);
}

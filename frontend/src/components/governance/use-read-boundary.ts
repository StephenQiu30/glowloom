"use client";
import { useEffect, useRef } from "react";
/** Cancel view-specific reads on filter/selection changes as well as unmount. */
export function useReadBoundary(key: string) {
  const current = useRef<AbortController | null>(null);
  useEffect(() => {
    const c = new AbortController();
    current.current = c;
    return () => c.abort();
  }, [key]);
  return async function read<T>(
    load: (signal: AbortSignal) => Promise<T>,
    apply: (value: T) => void,
  ) {
    const c = current.current;
    if (!c || c.signal.aborted) return;
    try {
      const value = await load(c.signal);
      if (!c.signal.aborted) apply(value);
    } catch (e) {
      if (!c.signal.aborted) throw e;
    }
  };
}

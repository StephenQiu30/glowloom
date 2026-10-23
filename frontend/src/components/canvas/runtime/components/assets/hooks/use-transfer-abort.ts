"use client";
import { useEffect, useRef, useState } from "react";

/** StrictMode's setup/cleanup probe keeps the live mount's requests; leaving cancels them. */
export function useTransferAbort() {
  const [controller] = useState(() => new AbortController());
  const mounted = useRef(false);
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      queueMicrotask(() => {
        if (!mounted.current) controller.abort();
      });
    };
  }, [controller]);
  return controller;
}

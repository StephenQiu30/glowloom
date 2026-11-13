"use client";
import { useEffect, useEffectEvent, useRef, useState } from "react";
/** Hide old scopes synchronously, retain same-scope refreshes, and discard aborted receipts. */
export function useInsightRead<T>(
  key: string,
  load: (signal: AbortSignal) => Promise<T>,
  revision = 0,
  onForbidden?: () => void,
) {
  const read = useEffectEvent(load);
  const reportForbidden = useEffectEvent(() => onForbidden?.());
  const controller = useRef<AbortController | null>(null);
  const [attempt, setAttempt] = useState(0);
  const generation = attempt + revision;
  const [state, setState] = useState<{
    key: string;
    generation: number;
    data: T | null;
    error: "" | "failed" | "forbidden";
  }>({ key, generation: -1, data: null, error: "" });
  useEffect(() => {
    const c = new AbortController();
    controller.current = c;
    read(c.signal)
      .then((data) => {
        if (!c.signal.aborted) setState({ key, generation, data, error: "" });
      })
      .catch((e: unknown) => {
        if (c.signal.aborted) return;
        const forbidden =
          typeof e === "object" &&
          e !== null &&
          "status" in e &&
          [401, 403].includes(Number(e.status));
        if (forbidden) reportForbidden();
        setState((s) => ({
          key,
          generation,
          data: forbidden ? null : s.key === key ? s.data : null,
          error: forbidden ? "forbidden" : "failed",
        }));
      });
    return () => c.abort();
  }, [key, generation]);
  const pending = state.key !== key || state.generation !== generation;
  return {
    key,
    data: state.key === key ? state.data : null,
    pending,
    error: pending ? "" : state.error,
    invalidate: () => {
      controller.current?.abort();
      setState({ key, generation, data: null, error: "forbidden" });
    },
    refresh: () => setAttempt((n) => n + 1),
  };
}

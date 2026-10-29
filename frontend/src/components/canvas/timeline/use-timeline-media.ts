"use client";
import { useEffect, useRef, useState, useSyncExternalStore } from "react";
import { getMediaPreview } from "@/api/media";
import { mediaError, requirePreview } from "@/components/media/library";
import {
  getScopeGeneration,
  getSessionGeneration,
  subscribeContextChange,
  RequestError,
} from "@/lib/request";
import type { TimelineMedia } from "./timeline-preview";
const contextIdentity = () =>
  `${getSessionGeneration()}/${getScopeGeneration()}`;

/** Owns expiring preview URLs for exactly one mounted timeline and identity. */
export function useTimelineMedia(
  projectID: string,
  scope: string,
  clipIDs: string,
) {
  const context = useSyncExternalStore(
    subscribeContextChange,
    contextIdentity,
    contextIdentity,
  );
  const key = `${context}/${projectID}/${scope}/${clipIDs}`;
  const retryRequest = useRef<{
    key: string;
    retry: (id?: string) => void;
  } | null>(null);
  const [state, setState] = useState<{
    key: string;
    media: Record<string, TimelineMedia>;
    errors: Record<string, string>;
  }>({ key: "", media: {}, errors: {} });
  useEffect(() => {
    const abort = new AbortController();
    const ids = JSON.parse(clipIDs) as string[];
    const refreshTimers = new Map<string, ReturnType<typeof setTimeout>>();
    const expiryTimers = new Map<string, ReturnType<typeof setTimeout>>();
    const generations = new Map<string, number>();
    const requestQueue: Array<() => void> = [];
    let activeRequests = 0;
    const startQueuedRequests = () => {
      while (activeRequests < 6 && requestQueue.length) {
        activeRequests++;
        requestQueue.shift()!();
      }
    };
    const current = () =>
      !abort.signal.aborted && contextIdentity() === context;
    const values: Record<string, TimelineMedia> = {};
    const failures: Record<string, string> = {};
    const publish = () => {
      if (current())
        setState({ key, media: { ...values }, errors: { ...failures } });
    };
    async function load(requested: string[]) {
      let index = 0;
      async function worker() {
        while (index < requested.length && current()) {
          const id = requested[index++];
          const generation = (generations.get(id) ?? 0) + 1;
          generations.set(id, generation);
          const accepts = () => current() && generations.get(id) === generation;
          clearTimeout(refreshTimers.get(id));
          try {
            await new Promise<void>((resolve) => {
              requestQueue.push(resolve);
              startQueuedRequests();
            });
            let value: Awaited<ReturnType<typeof getMediaPreview>>;
            try {
              if (!accepts()) continue;
              value = await getMediaPreview(
                { pid: projectID, asset_id: id },
                { signal: abort.signal },
              );
            } finally {
              activeRequests--;
              startQueuedRequests();
            }
            if (!accepts()) continue;
            if (value.asset?.project_id !== projectID)
              throw new Error("素材项目不匹配");
            const url = requirePreview(value, id);
            values[id] = {
              asset: value.asset,
              url,
              retryGeneration: generation,
            };
            delete failures[id];
            clearTimeout(expiryTimers.get(id));
            const remaining = Date.parse(value.expires_at!) - Date.now();
            expiryTimers.set(
              id,
              setTimeout(
                () => {
                  // A pending refresh does not extend the old URL's authorization.
                  if (!current()) return;
                  delete values[id];
                  failures[id] = "预览授权已过期，请重新读取素材";
                  publish();
                },
                Math.max(0, remaining),
              ),
            );
            refreshTimers.set(
              id,
              setTimeout(
                () => void load([id]),
                Math.max(1000, remaining - Math.min(30000, remaining / 10)),
              ),
            );
          } catch (cause) {
            if (!accepts()) continue;
            clearTimeout(expiryTimers.get(id));
            delete values[id];
            failures[id] =
              cause instanceof RequestError
                ? mediaError(cause)
                : cause instanceof Error &&
                    /[\u4e00-\u9fff]/.test(cause.message)
                  ? cause.message
                  : "素材读取失败，请重试。";
          }
          publish();
        }
      }
      await Promise.all(
        Array.from({ length: Math.min(requested.length, 6) }, worker),
      );
      if (current()) publish();
    }
    const retry = (id?: string) => {
      if (!current()) return;
      const requested = id ? ids.filter((value) => value === id) : ids;
      for (const assetID of requested) {
        clearTimeout(expiryTimers.get(assetID));
        clearTimeout(refreshTimers.get(assetID));
        delete values[assetID];
        delete failures[assetID];
      }
      publish();
      void load(requested);
    };
    retryRequest.current = { key, retry };
    void load(ids);
    return () => {
      abort.abort();
      if (retryRequest.current?.retry === retry) retryRequest.current = null;
      for (const timer of refreshTimers.values()) clearTimeout(timer);
      for (const timer of expiryTimers.values()) clearTimeout(timer);
    };
  }, [key, context, projectID, clipIDs]);
  return {
    media: state.key === key ? state.media : {},
    error:
      state.key === key
        ? [...new Set(Object.values(state.errors))].join("；")
        : "",
    errors: state.key === key ? state.errors : {},
    retry: (id?: string) => {
      if (retryRequest.current?.key === key) retryRequest.current.retry(id);
    },
  };
}

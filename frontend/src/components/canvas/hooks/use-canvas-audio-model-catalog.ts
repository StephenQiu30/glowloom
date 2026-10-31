"use client";

import { useEffect, useId, useRef, useSyncExternalStore } from "react";
import useSWR, { useSWRConfig, unstable_serialize } from "swr";
import {
  getScopeGeneration,
  getSessionGeneration,
  subscribeContextChange,
} from "@/lib/request";
import { readCanvasAudioModelCatalog } from "../lib/canvas-audio-model-catalog";
import type {
  CanvasAudioCatalogIdentity,
  CanvasAudioModelCatalog,
} from "../lib/canvas-audio-connection-capability";

const identityGeneration = () =>
  `${getSessionGeneration()}/${getScopeGeneration()}`;
/** Private consumer cache, scoped identity fence, and no stale capabilities during refresh. */
export function useCanvasAudioModelCatalog(
  identity: CanvasAudioCatalogIdentity,
) {
  const consumer = useId();
  const generation = useSyncExternalStore(
    subscribeContextChange,
    identityGeneration,
    () => "server",
  );
  const { cache } = useSWRConfig();
  const requests = useRef(new Map<string, Set<AbortController>>());
  const { projectID, scopeID } = identity;
  const key =
    projectID && scopeID
      ? ["canvas-audio-model-catalog", consumer, generation, projectID, scopeID]
      : null;
  const cacheKey = unstable_serialize(key);
  const query = useSWR(
    key,
    async () => {
      const controller = new AbortController();
      const active =
        requests.current.get(cacheKey) ?? new Set<AbortController>();
      active.add(controller);
      requests.current.set(cacheKey, active);
      try {
        return await readCanvasAudioModelCatalog(
          { projectID, scopeID },
          controller.signal,
        );
      } finally {
        active.delete(controller);
      }
    },
    {
      keepPreviousData: false,
      shouldRetryOnError: false,
      revalidateOnMount: true,
      revalidateOnFocus: true,
    },
  );
  useEffect(() => {
    const pending = requests.current;
    return () => {
      for (const request of pending.get(cacheKey) ?? []) request.abort();
      pending.delete(cacheKey);
      cache.delete(cacheKey);
    };
  }, [cache, cacheKey]);
  const catalog: CanvasAudioModelCatalog = query.error
    ? { projectID, scopeID, status: "error" }
    : !query.isValidating &&
        query.data?.projectID === projectID &&
        query.data.scopeID === scopeID
      ? query.data
      : { projectID, scopeID, status: "loading" };
  return { catalog, reload: () => query.mutate().catch(() => undefined) };
}

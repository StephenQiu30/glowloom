"use client";
import { useCallback } from "react";
import useSWR from "swr";
import { getMyPreferences, updateMyPreferences } from "@/api/identity";
import { getSessionGeneration } from "@/lib/request";

export type BackgroundMode = "dots" | "lines" | "blank";
export function backgroundIdentity(viewKey?: string) {
  if (!viewKey?.startsWith("glowloom:canvas-view:v1:")) return undefined;
  try {
    const parts = viewKey.split(":");
    const raw = decodeURIComponent(parts[parts.length === 7 ? 3 : 4]);
    return raw === "LOCAL" ? undefined : raw.replace(/^USER:/, "");
  } catch {
    return undefined;
  }
}
const preferenceKey = (identity: string) =>
  ["account-preferences", identity, getSessionGeneration()] as const;

/** Server-owned, session-scoped preferences; focus/poll reads other devices. */
export function useCanvasBackground(identity?: string) {
  const query = useSWR(
    identity ? preferenceKey(identity) : null,
    () => getMyPreferences(),
    {
      refreshInterval: 30_000,
      revalidateOnFocus: true,
      focusThrottleInterval: 1_000,
      shouldRetryOnError: false,
    },
  );
  const { data, mutate: updateCache } = query;
  const save = useCallback(
    async (next: BackgroundMode) => {
      if (!identity) throw new Error("请登录后修改画布背景。");
      const before = data ?? (await getMyPreferences());
      await updateMyPreferences({
        expected_revision: before.revision,
        canvas_background: next,
      });
      const after = await getMyPreferences();
      await updateCache(after, { revalidate: false });
      if (after.canvas_background !== next)
        throw new Error("背景已被其他设备更新，请重新选择。");
      return after;
    },
    [identity, data, updateCache],
  );
  return [query.data?.canvas_background ?? "dots", save, query] as const;
}

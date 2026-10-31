"use client";
import { useCallback, useEffect, useRef, useState } from "react";
import useSWR from "@/components/canvas/runtime/lib/use-live-query";
import { assetLibraryTarget } from "../library/asset-library-select";
import { useAccount } from "@/components/canvas/runtime/components/account/account-provider";
import { listMyAssets } from "@/components/canvas/runtime/api/asset-bff";
import { unwrap } from "@/components/canvas/runtime/lib/request";
import { SyncPendingError } from "@/components/canvas/runtime/lib/read-recovery";

type Snapshot = {
  state: "READY" | "SYNCING" | "UNAVAILABLE";
  page?: CanvasContract.AssetPageVO;
};
const changedEvent = "glowloom:asset-search:changed";

/** All entry points share authorization, visibility feedback and event-driven invalidation. */
export function useLibraryAssets(
  current = 1,
  pageSize = 12,
  keyword = "",
  kind = "all",
  onSuccess?: (data: CanvasContract.AssetPageVO) => void,
  scope = "ALL",
  enabled = true,
  referenceOnly = false,
  reviewStatus?: string,
) {
  const { account, ready, signingOut } = useAccount();
  const owner =
    ready && account && !signingOut ? String(account.id) : undefined;
  const key = JSON.stringify([
    owner,
    current,
    pageSize,
    keyword,
    kind,
    scope,
    enabled,
    referenceOnly,
    reviewStatus,
  ]);
  const operation = useRef(0);
  const [pending, setPending] = useState<string>();
  const target = {
    ...assetLibraryTarget(scope),
    ...(scope === "ALL" ? { libraryScope: "ALL" } : {}),
  };
  const query = useSWR<Snapshot>(
    owner && enabled
      ? [
          "my-assets",
          owner,
          current,
          pageSize,
          keyword,
          kind,
          scope,
          referenceOnly,
          reviewStatus,
        ]
      : null,
    async (_key, options) => {
      const page = unwrap(
        await listMyAssets(
          {
            current: String(current),
            pageSize: String(pageSize),
            ...target,
            keyword,
            kind,
            ...(referenceOnly ? { referenceOnly: true } : {}),
            ...(reviewStatus ? { reviewStatus } : {}),
          },
          options,
        ),
      );
      return { state: "READY", page };
    },
    {
      shouldRetryOnError: false,
      ...(onSuccess
        ? {
            onSuccess: (value: Snapshot) => {
              if (value.page) onSuccess(value.page);
            },
          }
        : {}),
    },
  );
  const { mutate: revalidate } = query;
  // A failed refresh never reverses a successful source write or rejects an upload callback.
  const refresh = useCallback(async () => {
    const token = ++operation.current;
    setPending(key);
    try {
      return await revalidate();
    } catch {
      return undefined;
    } finally {
      if (operation.current === token) {
        setPending(undefined);
      }
    }
  }, [key, revalidate]);
  useEffect(() => {
    if (!owner || !enabled) return;
    const read = () => {
      void refresh();
    };
    const changed = (event: Event) => {
      if ((event as CustomEvent<string>).detail === owner) read();
    };
    window.addEventListener(changedEvent, changed);
    return () => {
      window.removeEventListener(changedEvent, changed);
    };
  }, [owner, enabled, key, refresh]);
  const state = query.data?.page
    ? "READY"
    : query.error
      ? query.error instanceof SyncPendingError
        ? "SYNCING"
        : "UNAVAILABLE"
      : "READING";
  return {
    ...query,
    data: state === "READY" ? query.data?.page : undefined,
    syncState: state,
    isLoading: state === "READING",
    isRefreshing: pending === key || query.isValidating,
    mutate: refresh,
    changed: () => {
      if (owner)
        window.dispatchEvent(new CustomEvent(changedEvent, { detail: owner }));
    },
  };
}

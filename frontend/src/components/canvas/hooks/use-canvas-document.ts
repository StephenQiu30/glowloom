"use client";
import useSWR, { useSWRConfig } from "swr";
import { useCallback, useState } from "react";
import * as api from "@/components/canvas/runtime/api/canvas-document-bff";
import { useAccount } from "@/components/canvas/runtime/components/account/account-provider";
import { unwrap } from "@/components/canvas/runtime/lib/request";

export type CanvasScope = "USER" | "LOCAL";
// Share only active qualification reads. Completed authorization is never reused
// across mounts, and a newly authenticated account has a different object key.
const qualificationReads = new WeakMap<
  object,
  Map<string, Promise<CanvasContract.CanvasVO>>
>();

export function useCanvasDocument(
  scope: CanvasScope,
  canvasId: string,
  owner?: string | null,
  expectedProjectId?: string,
) {
  const { account, ready, signingOut } = useAccount();
  const { cache } = useSWRConfig();
  const identity =
    scope === "LOCAL" ? "LOCAL" : account ? `USER:${account.id}` : "guest";
  const enabled =
    scope === "LOCAL" ||
    (ready &&
      Boolean(account) &&
      !signingOut &&
      (owner === undefined || owner === identity));
  const [qualification, setQualification] = useState(() => ({
    enabled,
    key: crypto.randomUUID(),
    suspended: false,
    fresh: false,
  }));
  if (qualification.enabled !== enabled)
    setQualification({
      enabled,
      key: crypto.randomUUID(),
      suspended: false,
      fresh: false,
    });
  const invalidate = useCallback(() => {
    setQualification((current) => ({
      ...current,
      suspended: true,
      key: crypto.randomUUID(),
    }));
  }, []);
  const requalify = useCallback(() => {
    setQualification((current) => ({
      ...current,
      suspended: false,
      fresh: true,
      key: crypto.randomUUID(),
    }));
  }, []);
  const result = useSWR(
    enabled && !qualification.suspended
      ? [
          "canvas-document",
          identity,
          scope,
          canvasId,
          expectedProjectId ?? null,
          qualification.key,
        ]
      : null,
    () => {
      const owner = account ?? cache;
      let reads = qualificationReads.get(owner);
      if (!reads) {
        reads = new Map();
        qualificationReads.set(owner, reads);
      }
      const key = JSON.stringify([
        scope,
        canvasId,
        expectedProjectId,
        qualification.fresh ? qualification.key : null,
      ]);
      let pending = reads.get(key);
      if (!pending) {
        const currentReads = reads;
        pending = (
          scope === "LOCAL" ? api.localGetCanvas : api.getCanvasDocument
        )({ canvasId })
          .then(unwrap)
          .finally(() => currentReads.delete(key));
        reads.set(key, pending);
      }
      return pending;
    },
    {
      shouldRetryOnError: false,
      revalidateOnFocus: false,
      keepPreviousData: false,
    },
  );
  const qualified = Boolean(
    enabled &&
    !qualification.suspended &&
    result.data &&
    !result.error &&
    !result.isLoading &&
    result.data.access?.read !== false &&
    String(result.data.canvasId) === canvasId &&
    (!expectedProjectId || String(result.data.projectId) === expectedProjectId),
  );
  return {
    ...result,
    identity,
    ready,
    signedIn: Boolean(account),
    qualified,
    invalidate,
    requalify,
  };
}
export function useCanvasDocuments(
  scope: CanvasScope,
  query: CanvasContract.CanvasQueryRequest,
  owner?: string | null,
) {
  const { account, ready, signingOut } = useAccount();
  const identity =
    scope === "LOCAL" ? "LOCAL" : account ? `USER:${account.id}` : "guest";
  const enabled =
    scope === "LOCAL" ||
    (ready &&
      Boolean(account) &&
      !signingOut &&
      (owner === undefined || owner === identity));
  return {
    ...useSWR(
      enabled ? ["canvas-documents", identity, scope, query] : null,
      async () =>
        unwrap(
          await (scope === "LOCAL" ? api.localListCanvases : api.listCanvases)(
            query,
          ),
        ),
      { shouldRetryOnError: false, revalidateOnFocus: false },
    ),
    identity,
    ready,
    signedIn: Boolean(account),
  };
}

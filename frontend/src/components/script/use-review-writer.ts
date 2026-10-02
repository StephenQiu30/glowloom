"use client";
import { useEffect, useRef, useState } from "react";
import { ApiError } from "@/lib/request";
import { getWorkspace, requireScriptScope } from "./source-queries";
import { unknownSourceWrite, type ScriptScope } from "./source-intent";
import { runReviewIntent } from "./review-queries";
import {
  clearReviewIntent,
  loadReviewIntent,
  saveReviewIntent,
  reviewCommandSchema,
  reviewIntentSchema,
  type ReviewCommand,
  type ReviewIntent,
  type ReviewReceipt,
} from "./review-intent";
export function useReviewWriter(
  scope: ScriptScope,
  onConfirmed: (receipt: ReviewReceipt) => void | Promise<void>,
) {
  const [intent, setIntent] = useState<ReviewIntent | null>(null);
  const pending = useRef<ReviewIntent | null>(null);
  const active = useRef(false);
  const conflict = useRef(false);
  const [busy, setBusy] = useState(false);
  const [ready, setReady] = useState(false);
  const [error, setError] = useState<string>();
  const [storageError, setStorageError] = useState<string>();
  const [rejected, setRejected] = useState<{
    intent: ReviewIntent;
    cause: ApiError;
  }>();
  const { origin, actorId, orgId, projectId } = scope;
  useEffect(() => {
    const controller = new AbortController();
    let live = true;
    const current = { origin, actorId, orgId, projectId };
    void getWorkspace(projectId, controller.signal)
      .then((workspace) => {
        requireScriptScope(workspace, current);
        const original = loadReviewIntent(sessionStorage, current);
        if (live) {
          pending.current = original;
          setIntent(original);
          setReady(true);
        }
      })
      .catch((cause) => {
        if (live) {
          setStorageError(
            cause instanceof Error ? cause.message : "原审核读取失败。",
          );
          setReady(true);
        }
      });
    return () => {
      live = false;
      controller.abort();
    };
  }, [origin, actorId, orgId, projectId]);
  const hasPending = Boolean(intent);
  useEffect(() => {
    if (!hasPending && !storageError && !busy) return;
    const warn = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [hasPending, storageError, busy]);
  function remember(value: ReviewIntent | null) {
    pending.current = value;
    setIntent(value);
  }
  async function fresh() {
    if (window.location.origin !== origin)
      throw new ApiError(409, "scope_changed");
    requireScriptScope(await getWorkspace(projectId), scope);
  }
  async function execute(original: ReviewIntent, replay: boolean) {
    if (active.current) return;
    active.current = true;
    setBusy(true);
    setError(undefined);
    let sent = false;
    try {
      await fresh();
      const stored = loadReviewIntent(sessionStorage, scope);
      if (!stored || JSON.stringify(stored) !== JSON.stringify(original))
        throw new Error("原审核键与完整正文未持久一致，请恢复存储。");
      sent = true;
      const receipt = await runReviewIntent(original);
      clearReviewIntent(sessionStorage, scope);
      remember(null);
      setStorageError(undefined);
      try {
        await onConfirmed(receipt);
      } catch {
        setError("原审核已经确认，最新事实尚未读完。请重新读取当前事实。");
      }
    } catch (cause) {
      setError(
        cause instanceof Error ? cause.message : "审核尚未确认，请核验原键。",
      );
      if (
        sent &&
        !unknownSourceWrite(cause) &&
        !(
          replay &&
          cause instanceof ApiError &&
          [401, 403, 404].includes(cause.status)
        )
      ) {
        try {
          clearReviewIntent(sessionStorage, scope);
          remember(null);
          if (cause instanceof ApiError && cause.status === 409) {
            conflict.current = true;
            setRejected({ intent: original, cause });
          }
        } catch {
          setStorageError("原审核意图无法清理，请恢复存储后核验。");
        }
      } else if (!sent && !(cause instanceof ApiError))
        setStorageError(
          cause instanceof Error ? cause.message : "审核意图存储失败。",
        );
    } finally {
      active.current = false;
      setBusy(false);
    }
  }
  async function submit(command: ReviewCommand) {
    if (
      !ready ||
      active.current ||
      pending.current ||
      conflict.current ||
      storageError
    )
      return;
    const checked = reviewCommandSchema.parse(command);
    const original = reviewIntentSchema.parse({
      ...checked,
      ...scope,
      version: 1,
      key: crypto.randomUUID(),
    });
    remember(original);
    try {
      saveReviewIntent(sessionStorage, original);
    } catch (cause) {
      setStorageError(
        cause instanceof Error ? cause.message : "原审核存储失败。",
      );
      return;
    }
    await execute(original, false);
  }
  async function restoreStorage() {
    if (active.current) return;
    active.current = true;
    setBusy(true);
    try {
      await fresh();
      const stored = loadReviewIntent(sessionStorage, scope);
      const original = pending.current ?? stored;
      if (original) saveReviewIntent(sessionStorage, original);
      remember(original);
      setStorageError(undefined);
      setReady(true);
    } catch (cause) {
      setStorageError(
        cause instanceof Error ? cause.message : "原审核存储未恢复。",
      );
    } finally {
      active.current = false;
      setBusy(false);
    }
  }
  return {
    ready,
    intent,
    busy,
    error,
    storageError,
    rejected,
    locked: !ready || busy || Boolean(intent || storageError || rejected),
    submit,
    restoreStorage,
    replay: () =>
      pending.current && !storageError
        ? execute(pending.current, true)
        : Promise.resolve(),
    acknowledgeLatest: () => {
      conflict.current = false;
      setRejected(undefined);
      setError(undefined);
    },
  };
}

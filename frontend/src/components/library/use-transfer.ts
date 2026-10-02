"use client";
import { useEffect, useRef, useState } from "react";
import { ApiError } from "@/lib/request";
import { unknownLibraryWrite } from "@/components/media/library-intent";
import { freshLibrary } from "@/components/media/library-queries";
import type { LibraryIdentity } from "@/components/media/library-model";
import {
  clearTransferIntent,
  loadTransferIntent,
  saveTransferIntent,
  transferIntentSchema,
  type TransferIntent,
} from "./transfer-intent";
import { listTransfers, runTransferIntent } from "./transfer-queries";
import {
  transferCommandSchema,
  type TransferCommand,
  type TransferJob,
} from "./transfer-model";
export function useTransfer(
  identity: LibraryIdentity,
  onAccepted: (job: TransferJob) => void | Promise<void>,
) {
  const [intent, setIntent] = useState<TransferIntent | null>(null),
    pending = useRef<TransferIntent | null>(null),
    active = useRef(false);
  const [ready, setReady] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState<string>(),
    [storageError, setStorageError] = useState<string>(),
    [rejected, setRejected] = useState(false),
    [reviewed, setReviewed] = useState(false);
  const { origin, actorId, orgId, libraryId, scope } = identity,
    kind = scope.kind,
    projectId = scope.kind === "project" ? scope.project_id : undefined;
  useEffect(() => {
    const controller = new AbortController();
    let live = true;
    const current: LibraryIdentity = {
      origin,
      actorId,
      orgId,
      libraryId,
      scope: kind === "project" ? { kind, project_id: projectId! } : { kind },
    };
    void Promise.all([
      freshLibrary(current, controller.signal),
      listTransfers(current, 1, controller.signal),
    ])
      .then(() => {
        const saved = loadTransferIntent(sessionStorage, current);
        if (live) {
          pending.current = saved;
          setIntent(saved);
          setReady(true);
        }
      })
      .catch((cause) => {
        if (live) {
          setStorageError(
            cause instanceof Error ? cause.message : "迁移原意图无法读取。",
          );
          setReady(true);
        }
      });
    return () => {
      live = false;
      controller.abort();
    };
  }, [origin, actorId, orgId, libraryId, kind, projectId]);
  useEffect(() => {
    if (!intent && !busy && !storageError) return;
    const warn = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [intent, busy, storageError]);
  function remember(value: TransferIntent | null) {
    pending.current = value;
    setIntent(value);
  }
  async function execute(original: TransferIntent, replay: boolean) {
    if (active.current) return;
    active.current = true;
    setBusy(true);
    setError(undefined);
    setReviewed(false);
    let sent = false;
    try {
      await listTransfers(identity);
      const stored = loadTransferIntent(sessionStorage, identity);
      if (!stored || JSON.stringify(stored) !== JSON.stringify(original))
        throw new Error("迁移存储与原键/完整原正文不同，请先恢复存储。");
      sent = true;
      const receipt = await runTransferIntent(original);
      clearTransferIntent(sessionStorage, identity);
      remember(null);
      setStorageError(undefined);
      setRejected(false);
      try {
        await onAccepted(receipt);
      } catch {
        setError("原迁移请求已受理，但当前任务读取未完成，请重新读取。");
      }
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : "迁移尚未确认，请使用原键核验。",
      );
      if (
        sent &&
        !unknownLibraryWrite(cause) &&
        !(
          replay &&
          cause instanceof ApiError &&
          [401, 403, 404].includes(cause.status)
        )
      )
        setRejected(true);
      else if (!sent && !(cause instanceof ApiError))
        setStorageError(
          cause instanceof Error ? cause.message : "原迁移存储不可用。",
        );
    } finally {
      active.current = false;
      setBusy(false);
    }
  }
  async function submit(command: TransferCommand) {
    if (!ready || active.current || pending.current || storageError || rejected)
      return;
    try {
      const original = transferIntentSchema.parse({
        ...transferCommandSchema.parse(command),
        ...identity,
        version: 1,
        key: crypto.randomUUID(),
      });
      remember(original);
      saveTransferIntent(sessionStorage, original);
      await execute(original, false);
    } catch (cause) {
      setStorageError(
        cause instanceof Error ? cause.message : "迁移原键无法保存。",
      );
    }
  }
  async function restoreStorage() {
    if (active.current) return;
    active.current = true;
    setBusy(true);
    try {
      await Promise.all([freshLibrary(identity), listTransfers(identity)]);
      const stored = loadTransferIntent(sessionStorage, identity);
      if (
        pending.current &&
        stored &&
        JSON.stringify(stored) !== JSON.stringify(pending.current)
      )
        throw new Error("存储与当前迁移原意图不同，请保留原键后核验。");
      const original = pending.current ?? stored;
      if (original) saveTransferIntent(sessionStorage, original);
      remember(original);
      setReady(true);
      setStorageError(undefined);
    } catch (cause) {
      setStorageError(
        cause instanceof Error ? cause.message : "存储尚未恢复。",
      );
    } finally {
      active.current = false;
      setBusy(false);
    }
  }
  async function readLatest() {
    if (active.current) return;
    setBusy(true);
    active.current = true;
    try {
      await Promise.all([freshLibrary(identity), listTransfers(identity)]);
      setReviewed(true);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "当前事实不可读取。");
    } finally {
      setBusy(false);
      active.current = false;
    }
  }
  async function releaseRejected() {
    if (active.current || !rejected || !reviewed) return;
    try {
      await Promise.all([freshLibrary(identity), listTransfers(identity)]);
      clearTransferIntent(sessionStorage, identity);
      remember(null);
      setRejected(false);
      setReviewed(false);
      setError(undefined);
    } catch (cause) {
      setStorageError(
        cause instanceof Error ? cause.message : "原意图无法释放。",
      );
    }
  }
  return {
    ready,
    busy,
    error,
    storageError,
    intent,
    rejected,
    reviewed,
    locked: !ready || busy || Boolean(intent || storageError || rejected),
    submit,
    replay: () =>
      pending.current && !storageError
        ? execute(pending.current, true)
        : Promise.resolve(),
    restoreStorage,
    readLatest,
    releaseRejected,
  };
}
export type TransferWriter = ReturnType<typeof useTransfer>;

"use client";
import { useEffect, useRef, useState } from "react";
import { ApiError } from "@/lib/request";
import {
  clearLibraryIntent,
  libraryIntentSchema,
  loadLibraryIntent,
  saveLibraryIntent,
  unknownLibraryWrite,
  type LibraryIntent,
} from "./library-intent";
import {
  applyLibraryIntent,
  freshLibrary,
  getLibraryDetail,
} from "./library-queries";
import {
  libraryCommandSchema,
  type LibraryCommand,
  type LibraryDetail,
  type LibraryIdentity,
  type LibraryPage,
  type LibraryReceipt,
} from "./library-model";

export function useLibraryWriter(
  identity: LibraryIdentity,
  onAccepted: (receipt: LibraryReceipt) => void | Promise<void>,
) {
  const [intent, setIntent] = useState<LibraryIntent | null>(null),
    pending = useRef<LibraryIntent | null>(null),
    active = useRef(false);
  const [busy, setBusy] = useState(false),
    [ready, setReady] = useState(false),
    [error, setError] = useState<string>(),
    [storageError, setStorageError] = useState<string>();
  const [rejected, setRejected] = useState<{
      intent: LibraryIntent;
      cause: unknown;
    } | null>(null),
    [latest, setLatest] = useState<LibraryPage | null>(null);
  const [latestItems, setLatestItems] = useState<LibraryDetail[]>([]);
  const { origin, actorId, orgId, libraryId, scope } = identity;
  const kind = scope.kind,
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
    void freshLibrary(current, controller.signal)
      .then(() => {
        const stored = loadLibraryIntent(sessionStorage, current);
        if (live) {
          pending.current = stored;
          setIntent(stored);
          setReady(true);
        }
      })
      .catch((cause: unknown) => {
        if (live) {
          setStorageError(
            cause instanceof Error ? cause.message : "原意图读取失败。",
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
    if (!intent && !busy && !storageError && !rejected) return;
    const warn = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [intent, busy, storageError, rejected]);
  function remember(value: LibraryIntent | null) {
    pending.current = value;
    setIntent(value);
  }
  async function execute(original: LibraryIntent, replay: boolean) {
    if (active.current) return;
    active.current = true;
    setBusy(true);
    setError(undefined);
    setLatest(null);
    let sent = false;
    try {
      await freshLibrary(identity);
      const stored = loadLibraryIntent(sessionStorage, identity);
      if (!stored || JSON.stringify(stored) !== JSON.stringify(original))
        throw new Error("原键与完整正文的持久存储不一致，请先恢复存储。");
      sent = true;
      const receipt = await applyLibraryIntent(original);
      clearLibraryIntent(sessionStorage, identity);
      remember(null);
      setRejected(null);
      setStorageError(undefined);
      try {
        await onAccepted(receipt);
      } catch {
        setError("原修改已确认，但最新素材库读取未完成，请重新读取。");
      }
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : "结果尚未确认，请人工使用原键核验。",
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
        setRejected({ intent: original, cause });
    } finally {
      active.current = false;
      setBusy(false);
    }
  }
  async function submit(body: LibraryCommand) {
    if (active.current || !ready || pending.current || storageError || rejected)
      return;
    let original: LibraryIntent;
    try {
      original = libraryIntentSchema.parse({
        ...identity,
        version: 1,
        key: crypto.randomUUID(),
        body: libraryCommandSchema.parse(body),
      });
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "输入未通过校验。");
      return;
    }
    remember(original);
    try {
      saveLibraryIntent(sessionStorage, original);
    } catch (cause) {
      setStorageError(
        cause instanceof Error ? cause.message : "原意图无法持久保存。",
      );
      return;
    }
    await execute(original, false);
  }
  async function replay() {
    if (pending.current && !storageError) await execute(pending.current, true);
  }
  async function restoreStorage() {
    if (active.current) return;
    active.current = true;
    setBusy(true);
    try {
      await freshLibrary(identity);
      if (pending.current) saveLibraryIntent(sessionStorage, pending.current);
      else remember(loadLibraryIntent(sessionStorage, identity));
      setStorageError(undefined);
      setReady(true);
      setError(undefined);
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
    active.current = true;
    setBusy(true);
    setError(undefined);
    try {
      const original = pending.current?.body;
      const ids =
        original && "items" in original
          ? original.items.map((item) => item.id)
          : original?.action === "update_item"
            ? [original.item_id]
            : [];
      const page = await freshLibrary(identity);
      const items: LibraryDetail[] = [];
      let next = 0;
      await Promise.all(
        Array.from({ length: Math.min(8, ids.length) }, async () => {
          while (next < ids.length) {
            const index = next++;
            items[index] = await getLibraryDetail(identity, ids[index]);
          }
        }),
      );
      setLatest(page);
      setLatestItems(items);
    } catch (cause) {
      setLatest(null);
      setError(cause instanceof Error ? cause.message : "当前事实无法读取。");
    } finally {
      active.current = false;
      setBusy(false);
    }
  }
  async function discardRejected() {
    if (active.current || !rejected || !latest) return false;
    active.current = true;
    setBusy(true);
    try {
      await freshLibrary(identity);
      clearLibraryIntent(sessionStorage, identity);
      remember(null);
      setRejected(null);
      setLatest(null);
      setError(undefined);
      return true;
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "原意图不能释放。");
      return false;
    } finally {
      active.current = false;
      setBusy(false);
    }
  }
  return {
    ready,
    busy,
    intent,
    rejected,
    latest,
    latestItems,
    error,
    storageError,
    locked: !ready || busy || Boolean(intent || storageError || rejected),
    submit,
    replay,
    restoreStorage,
    readLatest,
    discardRejected,
  };
}
export type LibraryWriter = ReturnType<typeof useLibraryWriter>;

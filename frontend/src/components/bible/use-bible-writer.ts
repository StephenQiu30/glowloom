"use client";
import { useEffect, useRef, useState } from "react";
import { ApiError } from "@/lib/request";
import {
  bibleIntentSchema,
  clearBibleIntent,
  loadBibleIntent,
  saveBibleIntent,
  unknownBibleWrite,
  type BibleIntent,
} from "./bible-intent";
import {
  bibleCommandSchema,
  type BibleCommand,
  type BibleDetail,
  type BibleIdentity,
  type BiblePage,
  type BibleReceipt,
} from "./bible-model";
import {
  applyBibleIntent,
  freshBibleScope,
  getBibleDetail,
} from "./bible-queries";

type Latest = { scope: BiblePage; detail?: BibleDetail; target?: BibleDetail };
function message(cause: unknown) {
  return cause instanceof Error
    ? cause.message
    : "修改结果尚未确认，请保留原键与正文。";
}
export function useBibleWriter(
  identity: BibleIdentity,
  onAccepted: (receipt: BibleReceipt) => void | Promise<void>,
) {
  const pending = useRef<BibleIntent | null>(null),
    active = useRef(false);
  const [intent, setIntent] = useState<BibleIntent | null>(null),
    [ready, setReady] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState<string>(),
    [storageError, setStorageError] = useState<string>();
  const [rejected, setRejected] = useState<{
      intent: BibleIntent;
      cause: unknown;
    } | null>(null),
    [latest, setLatest] = useState<Latest | null>(null);
  const { origin, actorId, orgId, projectId } = identity;
  useEffect(() => {
    let live = true;
    const controller = new AbortController(),
      scope = { origin, actorId, orgId, projectId };
    void freshBibleScope(scope, controller.signal)
      .then(() => {
        const original = loadBibleIntent(sessionStorage, scope);
        if (live) {
          pending.current = original;
          setIntent(original);
          setReady(true);
        }
      })
      .catch((cause: unknown) => {
        if (live) {
          setStorageError(message(cause));
          setReady(true);
        }
      });
    return () => {
      live = false;
      controller.abort();
    };
  }, [origin, actorId, orgId, projectId]);
  useEffect(() => {
    if (!intent && !busy && !storageError && !rejected) return;
    const warn = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [intent, busy, storageError, rejected]);
  function remember(value: BibleIntent | null) {
    pending.current = value;
    setIntent(value);
  }
  async function execute(original: BibleIntent, replay: boolean) {
    if (active.current) return;
    active.current = true;
    setBusy(true);
    setError(undefined);
    setLatest(null);
    let sent = false;
    try {
      await freshBibleScope(identity);
      const stored = loadBibleIntent(sessionStorage, identity);
      if (!stored || JSON.stringify(stored) !== JSON.stringify(original))
        throw new Error("原键与完整输入的持久存储不一致，请先恢复存储。");
      sent = true;
      const receipt = await applyBibleIntent(original);
      clearBibleIntent(sessionStorage, identity);
      remember(null);
      setRejected(null);
      setStorageError(undefined);
      try {
        await onAccepted(receipt);
      } catch {
        setError("原修改已确认，最新设定读取未完成，请重新读取。");
      }
    } catch (cause) {
      setError(message(cause));
      if (
        sent &&
        !unknownBibleWrite(cause) &&
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
  async function submit(command: BibleCommand) {
    if (active.current || !ready || pending.current || storageError || rejected)
      return;
    let original: BibleIntent;
    try {
      original = bibleIntentSchema.parse({
        ...identity,
        version: 1,
        key: crypto.randomUUID(),
        command: bibleCommandSchema.parse(command),
      });
    } catch (cause) {
      setError(message(cause));
      return;
    }
    remember(original);
    try {
      saveBibleIntent(sessionStorage, original);
    } catch (cause) {
      setStorageError(message(cause));
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
      await freshBibleScope(identity);
      if (pending.current) saveBibleIntent(sessionStorage, pending.current);
      else remember(loadBibleIntent(sessionStorage, identity));
      setStorageError(undefined);
      setError(undefined);
      setReady(true);
    } catch (cause) {
      setStorageError(message(cause));
    } finally {
      active.current = false;
      setBusy(false);
    }
  }
  async function readLatest() {
    if (active.current) return;
    active.current = true;
    setBusy(true);
    setLatest(null);
    setError(undefined);
    try {
      const scope = await freshBibleScope(identity),
        command = pending.current?.command;
      const [detail, target] = await Promise.all([
        command?.id
          ? getBibleDetail(identity, command.kind, command.id)
          : undefined,
        command?.action === "merge"
          ? getBibleDetail(identity, "character", command.body.target_id)
          : undefined,
      ]);
      setLatest({ scope, detail, target });
    } catch (cause) {
      setError(message(cause));
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
      await freshBibleScope(identity);
      clearBibleIntent(sessionStorage, identity);
      remember(null);
      setRejected(null);
      setLatest(null);
      setError(undefined);
      return true;
    } catch (cause) {
      setError(message(cause));
      return false;
    } finally {
      active.current = false;
      setBusy(false);
    }
  }
  return {
    intent,
    ready,
    busy,
    error,
    storageError,
    rejected,
    latest,
    locked: !ready || busy || Boolean(intent || rejected || storageError),
    submit,
    replay,
    restoreStorage,
    readLatest,
    discardRejected,
  };
}
export type BibleWriter = ReturnType<typeof useBibleWriter>;

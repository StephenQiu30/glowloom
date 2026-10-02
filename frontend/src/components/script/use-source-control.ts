"use client";
import { useEffect, useRef, useState } from "react";
import { ApiError } from "@/lib/request";
import {
  clearSourceControl,
  loadSourceControl,
  saveSourceControl,
  sourceControlIntentSchema,
  type SourceControlIntent,
} from "./source-control-intent";
import { listSourceWrites, runSourceControl } from "./review-queries";
import {
  sourceControlCommandSchema,
  type SourceControlCommand,
} from "./source-write-model";
import { unknownSourceWrite, type ScriptScope } from "./source-intent";
export function useSourceControl(
  scope: ScriptScope,
  onAccepted: () => void | Promise<void>,
) {
  const [intent, setIntent] = useState<SourceControlIntent | null>(null);
  const pending = useRef<SourceControlIntent | null>(null);
  const active = useRef(false);
  const conflict = useRef(false);
  const [conflicted, setConflicted] = useState(false);
  const [busy, setBusy] = useState(false);
  const [ready, setReady] = useState(false);
  const [error, setError] = useState<string>();
  const [storageError, setStorageError] = useState<string>();
  const { projectId, origin, actorId, orgId } = scope;
  useEffect(() => {
    const controller = new AbortController();
    let live = true;
    const current = { projectId, origin, actorId, orgId };
    void listSourceWrites(current, 0, controller.signal)
      .then((page) => {
        if (page.current_actor_id !== actorId || page.current_org_id !== orgId)
          throw new ApiError(409, "scope_changed");
        const original = loadSourceControl(sessionStorage, current);
        if (live) {
          pending.current = original;
          setIntent(original);
          setReady(true);
        }
      })
      .catch((cause) => {
        if (live) {
          setStorageError(
            cause instanceof Error ? cause.message : "保存控制读取失败。",
          );
          setReady(true);
        }
      });
    return () => {
      live = false;
      controller.abort();
    };
  }, [projectId, origin, actorId, orgId]);
  useEffect(() => {
    if (!busy && !intent && !storageError) return;
    const warn = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [busy, intent, storageError]);
  function remember(value: SourceControlIntent | null) {
    pending.current = value;
    setIntent(value);
  }
  async function fresh() {
    if (window.location.origin !== scope.origin)
      throw new ApiError(409, "scope_changed");
    const page = await listSourceWrites(scope);
    if (
      page.current_actor_id !== scope.actorId ||
      page.current_org_id !== scope.orgId
    )
      throw new ApiError(409, "scope_changed");
  }
  async function execute(original: SourceControlIntent, replay: boolean) {
    if (active.current) return;
    active.current = true;
    setBusy(true);
    setError(undefined);
    let sent = false;
    try {
      await fresh();
      const stored = loadSourceControl(sessionStorage, scope);
      if (!stored || JSON.stringify(stored) !== JSON.stringify(original))
        throw new Error("存储中的保存控制与完整原意图不一致，请先恢复原存储。");
      sent = true;
      await runSourceControl(
        scope,
        {
          intentId: original.intentId,
          action: original.action,
          body: original.body,
        },
        original.key,
      );
      clearSourceControl(sessionStorage, scope);
      remember(null);
      setStorageError(undefined);
      try {
        await onAccepted();
      } catch {
        setError(
          "原控制已受理，当前状态读取未完成。请重新读取；受理不等于已停止或已清理。",
        );
      }
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : "控制尚未确认，请人工核验原键。",
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
          clearSourceControl(sessionStorage, scope);
          remember(null);
          conflict.current = cause instanceof ApiError && cause.status === 409;
          setConflicted(conflict.current);
        } catch {
          setStorageError("控制意图存储无法清理，请恢复存储后核验原键。");
        }
      } else if (!sent && !(cause instanceof ApiError))
        setStorageError(
          cause instanceof Error ? cause.message : "原控制存储失败。",
        );
    } finally {
      active.current = false;
      setBusy(false);
    }
  }
  async function submit(command: SourceControlCommand) {
    if (
      !ready ||
      active.current ||
      pending.current ||
      storageError ||
      conflict.current
    )
      return;
    const checked = sourceControlCommandSchema.parse(command);
    const original = sourceControlIntentSchema.parse({
      ...checked,
      ...scope,
      version: 1,
      key: crypto.randomUUID(),
    });
    remember(original);
    try {
      saveSourceControl(sessionStorage, original);
    } catch (cause) {
      setStorageError(
        cause instanceof Error ? cause.message : "控制意图存储失败。",
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
      const stored = loadSourceControl(sessionStorage, scope);
      const original = pending.current ?? stored;
      if (original) saveSourceControl(sessionStorage, original);
      remember(original);
      setReady(true);
      setStorageError(undefined);
    } catch (cause) {
      setStorageError(
        cause instanceof Error ? cause.message : "原存储未恢复。",
      );
    } finally {
      active.current = false;
      setBusy(false);
    }
  }
  return {
    ready,
    busy,
    intent,
    error,
    storageError,
    conflicted,
    locked: !ready || busy || Boolean(intent || storageError) || conflicted,
    submit,
    replay: () =>
      pending.current && !storageError
        ? execute(pending.current, true)
        : Promise.resolve(),
    restoreStorage,
    acknowledgeLatest: () => {
      conflict.current = false;
      setConflicted(false);
      setError(undefined);
    },
  };
}
